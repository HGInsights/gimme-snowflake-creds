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
	// Initialize configuration
	c config.Configuration

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
			token, err := okta.Auth(c)
			if err != nil {
				c.Logger.Debug("Unable to initiate the authentication flow", err)
			}

			// Write generic configuration
			if c.Profile.Generic {
				err = generator.WriteGenericCredentials(c, token)
				if err != nil {
					c.Logger.Debug("Unable to write generic configuration file", err)
				}
			}

			// Write ODBC configuration
			err = generator.WriteODBCConfig(c, token)
			if err != nil {
				c.Logger.Debug("Unable to write ODBC configuration file", err)
			}

			// Write DBT configuration
			err = generator.WriteDBTConfig(c, token)
			if err != nil {
				c.Logger.Debug("Unable to write DBT configuration file", err)
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
	rootCmd.Flags().StringVarP(&c.ProfileName, "profile", "p", "", "profile selection")
	rootCmd.Flags().StringVarP(&c.Profile.Account, "account", "a", "", "Snowflake account, like: xy12345.us-east-1")
	rootCmd.Flags().StringVarP(&c.ODBCDriverName, "driver-name", "z", "", "ODBC driver name")
	rootCmd.Flags().StringVarP(&c.ODBCDriverPath, "driver-path", "v", "", "ODBC driver path (local)")
	rootCmd.Flags().StringVarP(&c.Profile.Database, "database", "d", "", "Snowflake database")
	rootCmd.Flags().StringVarP(&c.Profile.Warehouse, "warehouse", "w", "", "Snowflake warehouse")
	rootCmd.Flags().StringVarP(&c.Profile.Schema, "schema", "x", "PUBLIC", "Snowflake schema")
	rootCmd.Flags().BoolVarP(&c.Profile.OAuth, "oauth", "", true, "enable/disable credential retrieval")
	rootCmd.Flags().BoolVarP(&c.Profile.Generic, "generic", "", false, "enable/disable generic credential setup")
	rootCmd.Flags().StringVarP(&c.Profile.OktaOrg, "okta-org", "o", "", "like: https://funtimes.oktapreview.com")
	rootCmd.Flags().StringVarP(&c.Profile.ODBCPath, "odbc-path", "n", "/etc", "Path containing odbc.ini")
	rootCmd.Flags().StringVarP(&c.Profile.ClientID, "client-id", "c", "", "OIDC Client ID of Okta application")
	rootCmd.Flags().StringVarP(&c.Profile.Role, "role", "s", "", "Snowflake role name")
	rootCmd.Flags().StringVarP(&c.Profile.IssuerURL, "issuer-url", "i", "", "issuer URL of Okta authorization server")
	rootCmd.Flags().StringVarP(&c.Profile.RedirectURI, "redirect-uri", "r", "", "redirect URI of Okta application")
	rootCmd.Flags().StringVarP(&c.Profile.Username, "username", "u", "", "username for Okta")
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
	c.Logger = log

	// Read in configuration
	if err := v.ReadInConfig(); err == nil {
		c.Logger.Debug(v.ConfigFileUsed())
	}

	// Set profile to default profile if no profile argument is passed
	if c.ProfileName == "" {
		c.ProfileName = c.DefaultProfile
	}

	// Unmarshal configuration into configuration struct
	err = v.Unmarshal(&c)
	if err != nil {
		c.Logger.Error("error", err)
		os.Exit(0)
	}

	// Unmarshal profile into profile struct
	err = v.UnmarshalKey(c.ProfileName, &c.Profile)
	if err != nil {
		c.Logger.Error("error", err)
		os.Exit(0)
	}

	// Load default parameters
	c.HomeDir = home
	config.LoadDefaults(&c)
	err = config.ValidateConfiguration(&c)
	if err != nil {
		c.Logger.Debug("error", err)
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
