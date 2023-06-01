package config

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/HGInsights/gimme-snowflake-creds/pkg/utils"
	"github.com/go-playground/validator"
	"github.com/hashicorp/go-hclog"
)

var (
	validate *validator.Validate

	GlobalParams = []string{
		"default",
		"driver-name",
		"driver-path",
	}

	oauthParams = []string{
		"okta-org",
		"client-id",
		"issuer-url",
		"redirect-uri",
	}

	colorGreen = "\033[32m"
	colorRed   = "\033[31m"
)

type Configuration struct {
	DefaultProfile string
	ODBCDriverName string `mapstructure:"driver-name" validate:"required"`
	ODBCDriverPath string `mapstructure:"driver-path" validate:"required"`
	ProfileName    string
	Profile        Profile
	Forget         bool
	HomeDir        string
	Logger         hclog.Logger
	ColorSuccess   string
	ColorFailure   string
}

type Profile struct {
	OAuth       bool   `mapstructure:"oauth"`
	Generic     bool   `mapstructure:"generic"`
	Account     string `mapstructure:"account" validate:"required"`
	Database    string `mapstructure:"database" validate:"required"`
	Warehouse   string `mapstructure:"warehouse" validate:"required"`
	Schema      string `mapstructure:"schema"`
	DbtProfile  string `mapstructure:"dbt-profile"`
	ThreadCount uint64 `mapstructure:"threads"`
	KeepAlive   bool   `mapstructure:"client_session_keep_alive"`
	OktaOrg     string `mapstructure:"okta-org" validate:"required,url"`
	ODBCPath    string `mapstructure:"odbc-path" validate:"required"`
	ClientID    string `mapstructure:"client-id" validate:"required"`
	Role        string `mapstructure:"role" validate:"required"`
	IssuerURL   string `mapstructure:"issuer-url" validate:"required,url"`
	RedirectURI string `mapstructure:"redirect-uri" validate:"required,uri"`
	Username    string `mapstructure:"username" validate:"required,email"`
	Password    string
}

type Credentials struct {
	ExpiresIn   int
	AccessToken string
}

func LoadDefaults(c *Configuration) error {
	c.ColorSuccess = colorGreen
	c.ColorFailure = colorRed

	// Determine if we're running in Docker
	if utils.InDocker() {
		c.Logger.Debug("Running in Docker!")
		c.Profile.ODBCPath = "/root/Library/ODBC"
	}

	return nil
}

func ValidateConfiguration(c *Configuration) error {
	validate = validator.New()

	// Register function to get tag name from mapstructure tags
	validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("mapstructure"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})

	err := validate.Struct(c)
	if err != nil {
		for _, err := range err.(validator.ValidationErrors) {
			if !c.Profile.OAuth && utils.Contains(oauthParams, err.Field()) {
				return nil
			} else {
				fmt.Println(string(c.ColorFailure), "Parameter", err.Field(), "is required")
			}
		}

		return err
	}

	return nil
}
