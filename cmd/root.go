package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/HGInsights/gimme-snowflake-creds/internal/config"
	"github.com/HGInsights/gimme-snowflake-creds/pkg/generator"
	"github.com/HGInsights/gimme-snowflake-creds/pkg/okta"
	"github.com/hashicorp/go-hclog"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/viper"
)

var (
	// Initial variables
	p config.Configuration

	rootCmd = &cobra.Command{
		Use:   "gimme-snowflake-creds",
		Args:  cobra.NoArgs,
		Short: "Okta --> OAuth --> Snowflake --> Creds",
		Long:  `A tool that utilizes Okta IdP via OAuth to acquire temporary Snowflake credentials`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Load configuration
			return initConfig(cmd)
		},
		Run: func(cmd *cobra.Command, args []string) {
			// Initialize authentication flow
			token, err := okta.Auth(p)
			if err != nil {
				p.Logger.Debug("Unable to initiate the authentication flow", err)
			}

			// Write ODBC configuration
			err = generator.WriteODBCConfig(p, token)
			if err != nil {
				p.Logger.Debug("Unable to write ODBC configuration file", err)
			}

			// Write DBT configuration
			err = generator.WriteDBTConfig(p, token)
			if err != nil {
				p.Logger.Debug("Unable to write DBT configuration file", err)
			}
		},
	}
)

func Execute() {
	cobra.CheckErr(rootCmd.Execute())

	// CTRL+C catcher
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		os.Exit(0)
	}()
}

func init() {
	// Set flags
	rootCmd.Flags().StringVarP(&p.Profile, "profile", "p", "dev", "profile selection")
	rootCmd.Flags().StringVarP(&p.Account, "account", "a", "", "Snowflake account, like: xy12345.us-east-1")
	rootCmd.Flags().StringVarP(&p.Database, "database", "d", "", "Snowflake database")
	rootCmd.Flags().StringVarP(&p.Warehouse, "warehouse", "w", "", "Snowflake warehouse")
	rootCmd.Flags().StringVarP(&p.Schema, "schema", "x", "PUBLIC", "Snowflake schema")
	rootCmd.Flags().StringVarP(&p.OktaOrg, "okta-org", "o", "", "like: https://funtimes.oktapreview.com")
	rootCmd.Flags().StringVarP(&p.ODBCini, "odbc-ini", "n", "/etc/odbc.ini", "Location of odbc.ini")
	rootCmd.Flags().StringVarP(&p.ODBCDriver, "odbc-driver", "v", "", "Location of ODBC driver")
	rootCmd.Flags().StringVarP(&p.ClientID, "client-id", "c", "", "OIDC Client ID of Okta application")
	rootCmd.Flags().StringVarP(&p.Role, "role", "s", "", "space separated list of Snowflake role names")
	rootCmd.Flags().StringVarP(&p.IssuerURL, "issuer-url", "i", "", "issuer URL of Okta authorization server")
	rootCmd.Flags().StringVarP(&p.RedirectURI, "redirect-uri", "r", "", "redirect URI of Okta application")
	rootCmd.Flags().StringVarP(&p.Username, "username", "u", "", "username for Okta")
}

// initConfig reads in config file and ENV variables if set.
func initConfig(cmd *cobra.Command) error {
	// Find home directory.
	home, err := homedir.Dir()
	cobra.CheckErr(err)

	// Initialize Viper
	v := viper.New()

	// Search config in home directory with name ".okta_snowflake_login_config" (without extension).
	v.AddConfigPath(home)
	v.SetConfigType("yaml")
	v.SetConfigName(".okta_snowflake_login_config")

	v.SetEnvPrefix("GSC") // Set environment variable prefix
	v.AutomaticEnv()      // Read in environment variables that match

	// Configure logging
	logLevel := v.GetString("LOG")
	if logLevel == "" {
		logLevel = "INFO"
	}
	log := hclog.New(&hclog.LoggerOptions{
		Level: hclog.LevelFromString(logLevel),
	})
	p.Logger = log

	// Read in configuration
	if err := v.ReadInConfig(); err == nil {
		p.Logger.Debug(v.ConfigFileUsed())
	}

	// Unmarshal configuration into configuration struct
	err = v.UnmarshalKey(p.Profile, &p)
	if err != nil {
		p.Logger.Error("error", err)
		os.Exit(0)
	}

	// Load default parameters
	p.HomeDir = home
	config.LoadDefaults(&p)
	err = config.ValidateConfiguration(&p)
	if err != nil {
		p.Logger.Debug("error", err)
		os.Exit(0)
	}

	// Bind flags between Viper and Cobra
	bindFlags(cmd, v)

	return nil
}

func bindFlags(cmd *cobra.Command, v *viper.Viper) {
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		v.BindPFlag(f.Name, f)

		if !f.Changed && v.IsSet(f.Name) {
			value := v.Get(f.Name)
			cmd.Flags().Set(f.Name, fmt.Sprintf("%v", value))
		}
	})
}
