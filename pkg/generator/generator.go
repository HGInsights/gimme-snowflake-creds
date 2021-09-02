package generator

import (
	"fmt"
	"os"

	"github.com/HGInsights/gimme-snowflake-creds/internal/config"
	"github.com/spf13/viper"
	"gopkg.in/ini.v1"
)

func WriteGenericCredentials(p config.Configuration, t *config.Credentials) error {
	var genericConfigPath = p.HomeDir + "/.gsc/" + p.Profile
	var genericConfigFile = genericConfigPath + "/credentials"

	// Ensure `~/.gsc` directory exists
	if _, err := os.Stat(genericConfigPath); os.IsNotExist(err) {
		p.Logger.Debug("Couldn't find existing generic configuration path, creating...", "error", err)
		os.MkdirAll(genericConfigPath, os.ModePerm)
	}

	generic, err := ini.Load(genericConfigFile)
	if err != nil {
		fmt.Println(string(p.ColorSuccess), "Generic: No existing configuration found, creating file...")
		p.Logger.Debug("Couldn't read existing generic config", "error", err)
		generic = ini.Empty()
	}

	generic.Section("").Key("SNOWFLAKE_USER").SetValue(p.Username)
	generic.Section("").Key("SNOWFLAKE_OAUTH_ACCESS_TOKEN").SetValue(t.AccessToken)

	err = generic.SaveTo(genericConfigFile)
	if err != nil {
		fmt.Println(string(p.ColorFailure), "Generic: Couldn't write config!")
		p.Logger.Debug("Couldn't write generic config", "error", err)
		return err
	}

	fmt.Println(string(p.ColorSuccess), "Generic: Configuration written to:", genericConfigFile)

	return nil
}

func WriteODBCConfig(p config.Configuration, t *config.Credentials) error {
	var odbcConfigFile = p.ODBCPath + "/odbc.ini"
	var serverURL = p.Account + ".snowflakecomputing.com"

	// Ensure ODBC path defined by user exists
	if _, err := os.Stat(p.ODBCPath); os.IsNotExist(err) {
		p.Logger.Debug("Couldn't find existing ODBC path, creating...", "error", err)
		os.Mkdir(p.ODBCPath, os.ModePerm)
	}

	odbc, err := ini.Load(odbcConfigFile)
	if err != nil {
		fmt.Println(string(p.ColorSuccess), "ODBC: No existing configuration found, creating file...")
		p.Logger.Debug("Couldn't read existing ODBC config", "error", err)
		odbc = ini.Empty()
	}

	odbc.Section(p.Profile).Key("Driver").SetValue(p.ODBCDriver)
	odbc.Section(p.Profile).Key("server").SetValue(serverURL)
	odbc.Section(p.Profile).Key("uid").SetValue(p.Username)
	odbc.Section(p.Profile).Key("role").SetValue(p.Role)
	odbc.Section(p.Profile).Key("database").SetValue(p.Database)
	odbc.Section(p.Profile).Key("schema").SetValue(p.Schema)
	odbc.Section(p.Profile).Key("warehouse").SetValue(p.Warehouse)

	if p.OAuth {
		odbc.Section(p.Profile).Key("authenticator").SetValue("oauth")
		odbc.Section(p.Profile).Key("token").SetValue(t.AccessToken)
	} else {
		odbc.Section(p.Profile).Key("authenticator").SetValue("externalbrowser")
	}

	err = odbc.SaveTo(odbcConfigFile)
	if err != nil {
		fmt.Println(string(p.ColorFailure), "ODBC: Couldn't write config!")
		p.Logger.Debug("Couldn't write ODBC config", "error", err)
		return err
	}

	fmt.Println(string(p.ColorSuccess), "ODBC: Configuration written to:", odbcConfigFile)

	return nil
}

func WriteDBTConfig(p config.Configuration, t *config.Credentials) error {
	var dbtConfigPath = p.HomeDir + "/.dbt"
	var dbtConfigFile = dbtConfigPath + "/profiles.yml"

	var dbt = viper.New()
	dbt.SetConfigFile(dbtConfigFile)
	dbt.Set("default.target", p.Default)

	// Ensure DBT configuration directory exists
	if _, err := os.Stat(dbtConfigPath); os.IsNotExist(err) {
		p.Logger.Debug("Couldn't find existing DBT configuration directory, creating...", "error", err)
		os.Mkdir(dbtConfigPath, os.ModePerm)
	}

	if p.OAuth {
		profile := map[string]interface{}{
			string(p.Profile): map[string]interface{}{
				"type":                      "snowflake",
				"account":                   p.Account,
				"user":                      p.Username,
				"authenticator":             "oauth",
				"token":                     t.AccessToken,
				"role":                      p.Role,
				"database":                  p.Database,
				"warehouse":                 p.Warehouse,
				"schema":                    p.Schema,
				"threads":                   10,
				"client_session_keep_alive": false,
			},
		}

		dbt.Set("default.outputs", profile)
	} else {
		profile := map[string]interface{}{
			string(p.Profile): map[string]interface{}{
				"type":                      "snowflake",
				"account":                   p.Account,
				"user":                      p.Username,
				"authenticator":             "externalbrowser",
				"role":                      p.Role,
				"database":                  p.Database,
				"warehouse":                 p.Warehouse,
				"schema":                    p.Schema,
				"threads":                   10,
				"client_session_keep_alive": false,
			},
		}

		dbt.Set("default.outputs", profile)
	}

	err := dbt.ReadInConfig()
	if err != nil {
		fmt.Println(string(p.ColorSuccess), "DBT: No existing configuration found, creating file...")
		p.Logger.Debug("Couldn't read existing DBT config", "error", err)
	}
	err = dbt.WriteConfig()
	if err != nil {
		fmt.Println(string(p.ColorFailure), "DBT: Couldn't write config!")
		p.Logger.Debug("Couldn't write DBT config", "error", err)
		return err
	}

	fmt.Println(string(p.ColorSuccess), "DBT: Configuration written to:", dbtConfigFile)

	return nil
}
