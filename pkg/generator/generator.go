package generator

import (
	"fmt"
	"os"

	"github.com/HGInsights/gimme-snowflake-creds/internal/config"
	"github.com/spf13/viper"
	"gopkg.in/ini.v1"
)

func WriteGenericCredentials(c config.Configuration, t *config.Credentials) error {
	var genericConfigPath = c.HomeDir + "/.gsc/" + c.ProfileName
	var genericConfigFile = genericConfigPath + "/credentials"

	// Ensure `~/.gsc` directory exists
	if _, err := os.Stat(genericConfigPath); os.IsNotExist(err) {
		c.Logger.Debug("Couldn't find existing generic configuration path, creating...", "error", err)
		os.MkdirAll(genericConfigPath, os.ModePerm)
	}

	generic, err := ini.Load(genericConfigFile)
	if err != nil {
		fmt.Println(string(c.ColorSuccess), "Generic: No existing configuration found, creating file...")
		c.Logger.Debug("Couldn't read existing generic config", "error", err)
		generic = ini.Empty()
	}

	generic.Section("").Key("SNOWFLAKE_USER").SetValue(c.Profile.Username)
	generic.Section("").Key("SNOWFLAKE_OAUTH_ACCESS_TOKEN").SetValue(t.AccessToken)

	err = generic.SaveTo(genericConfigFile)
	if err != nil {
		fmt.Println(string(c.ColorFailure), "Generic: Couldn't write config!")
		c.Logger.Debug("Couldn't write generic config", "error", err)
		return err
	}

	fmt.Println(string(c.ColorSuccess), "Generic: Configuration written to:", genericConfigFile)

	return nil
}

func WriteODBCConfig(c config.Configuration, t *config.Credentials) error {
	var odbcConfigFile = c.Profile.ODBCPath + "/odbc.ini"
	var serverURL = c.Profile.Account + ".snowflakecomputing.com"

	// Ensure ODBC path defined by user exists
	if _, err := os.Stat(c.Profile.ODBCPath); os.IsNotExist(err) {
		c.Logger.Debug("Couldn't find existing ODBC path, creating...", "error", err)
		os.Mkdir(c.Profile.ODBCPath, os.ModePerm)
	}

	odbc, err := ini.Load(odbcConfigFile)
	if err != nil {
		fmt.Println(string(c.ColorSuccess), "ODBC: No existing configuration found, creating file...")
		c.Logger.Debug("Couldn't read existing ODBC config", "error", err)
		odbc = ini.Empty()
	}

	// Create driver alias
	odbc.Section(c.ODBCDriverName).Key("Driver").SetValue(c.ODBCDriverPath)

	// Create profile DSN
	odbc.Section(c.ProfileName).Key("Driver").SetValue(c.ODBCDriverName)
	odbc.Section(c.ProfileName).Key("server").SetValue(serverURL)
	odbc.Section(c.ProfileName).Key("uid").SetValue(c.Profile.Username)
	odbc.Section(c.ProfileName).Key("role").SetValue(c.Profile.Role)
	odbc.Section(c.ProfileName).Key("database").SetValue(c.Profile.Database)
	odbc.Section(c.ProfileName).Key("schema").SetValue(c.Profile.Schema)
	odbc.Section(c.ProfileName).Key("warehouse").SetValue(c.Profile.Warehouse)

	if c.Profile.OAuth {
		odbc.Section(c.ProfileName).Key("authenticator").SetValue("oauth")
		odbc.Section(c.ProfileName).Key("token").SetValue(t.AccessToken)
	} else {
		odbc.Section(c.ProfileName).Key("authenticator").SetValue("externalbrowser")
	}

	err = odbc.SaveTo(odbcConfigFile)
	if err != nil {
		fmt.Println(string(c.ColorFailure), "ODBC: Couldn't write config!")
		c.Logger.Debug("Couldn't write ODBC config", "error", err)
		return err
	}

	fmt.Println(string(c.ColorSuccess), "ODBC: Configuration written to:", odbcConfigFile)

	return nil
}

func WriteDBTConfig(c config.Configuration, t *config.Credentials) error {
	var dbtConfigPath = c.HomeDir + "/.dbt"
	var dbtConfigFile = dbtConfigPath + "/profiles.yml"

	var dbt = viper.New()
	dbt.SetConfigFile(dbtConfigFile)
	dbt.Set("default.target", c.DefaultProfile)

	// Ensure DBT configuration directory exists
	if _, err := os.Stat(dbtConfigPath); os.IsNotExist(err) {
		c.Logger.Debug("Couldn't find existing DBT configuration directory, creating...", "error", err)
		os.Mkdir(dbtConfigPath, os.ModePerm)
	}

	if c.Profile.OAuth {
		profile := map[string]interface{}{
			string(c.ProfileName): map[string]interface{}{
				"type":                      "snowflake",
				"account":                   c.Profile.Account,
				"user":                      c.Profile.Username,
				"authenticator":             "oauth",
				"token":                     t.AccessToken,
				"role":                      c.Profile.Role,
				"database":                  c.Profile.Database,
				"warehouse":                 c.Profile.Warehouse,
				"schema":                    c.Profile.Schema,
				"threads":                   10,
				"client_session_keep_alive": false,
			},
		}

		dbt.Set("default.outputs", profile)
	} else {
		profile := map[string]interface{}{
			string(c.ProfileName): map[string]interface{}{
				"type":                      "snowflake",
				"account":                   c.Profile.Account,
				"user":                      c.Profile.Username,
				"authenticator":             "externalbrowser",
				"role":                      c.Profile.Role,
				"database":                  c.Profile.Database,
				"warehouse":                 c.Profile.Warehouse,
				"schema":                    c.Profile.Schema,
				"threads":                   10,
				"client_session_keep_alive": false,
			},
		}

		dbt.Set("default.outputs", profile)
	}

	err := dbt.ReadInConfig()
	if err != nil {
		fmt.Println(string(c.ColorSuccess), "DBT: No existing configuration found, creating file...")
		c.Logger.Debug("Couldn't read existing DBT config", "error", err)
	}
	err = dbt.WriteConfig()
	if err != nil {
		fmt.Println(string(c.ColorFailure), "DBT: Couldn't write config!")
		c.Logger.Debug("Couldn't write DBT config", "error", err)
		return err
	}

	fmt.Println(string(c.ColorSuccess), "DBT: Configuration written to:", dbtConfigFile)

	return nil
}
