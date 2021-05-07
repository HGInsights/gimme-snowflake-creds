package generator

import (
	"fmt"

	"github.com/HGInsights/gimme-snowflake-creds/internal/config"
	"github.com/spf13/viper"
	"gopkg.in/ini.v1"
)

func WriteODBCConfig(p config.Configuration, t *config.Credentials) error {
	var serverURL = p.Account + ".snowflakecomputing.com"

	odbc, err := ini.Load(p.ODBCini)
	if err != nil {
		fmt.Println(string(p.ColorSuccess), "ODBC: No existing configuration found, is a driver installed?")
		p.Logger.Debug("Couldn't read ODBC config", "error", err)
		return err
	}

	odbc.Section(p.Profile).Key("Driver").SetValue(p.ODBCDriver)
	odbc.Section(p.Profile).Key("server").SetValue(serverURL)
	odbc.Section(p.Profile).Key("uid").SetValue(p.Username)
	odbc.Section(p.Profile).Key("database").SetValue(p.Database)
	odbc.Section(p.Profile).Key("warehouse").SetValue(p.Warehouse)
	odbc.Section(p.Profile).Key("role").SetValue(p.Role)
	odbc.Section(p.Profile).Key("authenticator").SetValue("oauth")
	odbc.Section(p.Profile).Key("token").SetValue(t.AccessToken)

	err = odbc.SaveTo(p.ODBCini)
	if err != nil {
		fmt.Println(string(p.ColorFailure), "ODBC: Couldn't write config!")
		p.Logger.Debug("Couldn't write ODBC config", "error", err)
		return err
	}

	fmt.Println(string(p.ColorSuccess), "ODBC: Configuration written to:", p.ODBCini)

	return nil
}

func WriteDBTConfig(p config.Configuration, t *config.Credentials) error {
	var dbtConfigFile = p.HomeDir + "/.dbt/profiles.yml"

	var dbt = viper.New()
	dbt.SetConfigFile(dbtConfigFile)

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

	dbt.Set("default.target", "dev")
	dbt.Set("default.outputs", profile)

	err := dbt.ReadInConfig()
	if err != nil {
		fmt.Println(string(p.ColorSuccess), "DBT: No existing configuration found, creating file...")
		p.Logger.Debug("Couldn't read DBT config", "error", err)
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
