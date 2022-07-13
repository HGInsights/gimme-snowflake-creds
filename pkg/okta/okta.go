package okta

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/HGInsights/gimme-snowflake-creds/internal/config"
	"github.com/HGInsights/gimme-snowflake-creds/pkg/verifier"
	"github.com/google/uuid"
	"github.com/manifoldco/promptui"
	"github.com/zalando/go-keyring"
)

type authnResponse struct {
	Status       string `json:"status"`
	StateToken   string `json:"stateToken"`
	SessionToken string `json:"sessionToken"`
	Embedded     struct {
		Factors []factor `json:"factors"`
	} `json:"_embedded"`
}

type factor struct {
	FactorType string `json:"factorType"`
	Provider   string `json:"provider"`
	Links      struct {
		Verify struct {
			VerifyURL string `json:"href"`
		} `json:"verify"`
	} `json:"_links"`
}

type verifyResponse struct {
	Status       string `json:"status"`
	FactorResult string `json:"factorResult"`
	ExpiresAt    string `json:"expiresAt"`
	SessionToken string `json:"sessionToken"`
}

type authorizeResponse struct {
	State        string
	Code         string
	CodeVerifier string
}

type tokenResponse struct {
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
	AccessToken string `json:"access_token"`
}

func Auth(c config.Configuration) (*config.Credentials, error) {
	p := new(config.Credentials)

	if c.Profile.OAuth {
		// Retrieve password for configured user
		c, err := retrievePassword(c)
		if err != nil {
			c.Logger.Debug("Unable to return password prompt input", "error", err)
			os.Exit(0)
		}

		// Perform primary, initial authentication
		authn, err := primaryAuth(c)
		if err != nil {
			c.Logger.Debug("Unable to return primary authentication response", "error", err)
			os.Exit(0)
		}

		if authn.Status == "SUCCESS" {
			// Retrieve OAuth token
			token, err := oauthToken(c, nil)
			if err != nil {
				c.Logger.Debug("Unable to return OAuth token", "error", err)
				os.Exit(0)
			}

			p.ExpiresIn = token.ExpiresIn
			p.AccessToken = token.AccessToken

			return p, nil
		} else if authn.Status == "MFA_REQUIRED" {
			// Prompt for factor type
			factor, err := factorSelect(c, authn)
			if err != nil {
				c.Logger.Debug("Unable to return factor selection", "error", err)
				os.Exit(0)
			}

			err = factorPush(c, authn, factor)
			if err != nil {
				c.Logger.Debug("Unable to initiate factor push", "error", err)
				os.Exit(0)
			}

			// Prompt for factor challenge
			challenge, err := factorChallenge(c, factor)
			if err != nil {
				c.Logger.Debug("Unable to return factor challenge", "error", err)
				os.Exit(0)
			}

			// Perform MFA verification
			verify, err := verifyMFA(c, authn, factor, challenge)
			for verify.FactorResult == "WAITING" {
				c.Logger.Debug("Checking MFA verification...")
				time.Sleep(1 * time.Second)
				verify, err = verifyMFA(c, authn, factor, challenge)
			}
			if verify.FactorResult == "REJECTED" {
				fmt.Println(string(c.ColorFailure), "MFA challenge rejected!")
				os.Exit(0)
			} else if verify.FactorResult == "TIMEOUT" {
				fmt.Println(string(c.ColorSuccess), "MFA challenge timed out!")
				os.Exit(0)
			} else if verify.Status == "SUCCESS" {
				fmt.Println(string(c.ColorSuccess), "MFA verified!")
			}
			if err != nil {
				fmt.Println(string(c.ColorFailure), "MFA verification failed!")
				c.Logger.Debug("Unable to return MFA verifcation", "error", err)
				os.Exit(0)
			}

			// Retrieve authorizataion code
			auth, err := authCode(c, verify)
			if err != nil {
				c.Logger.Debug("Unable to return authorization code", "error", err)
				os.Exit(0)
			}

			// Retrieve OAuth token
			token, err := oauthToken(c, auth)
			if err != nil {
				c.Logger.Debug("Unable to return OAuth token", "error", err)
				os.Exit(0)
			}

			p.ExpiresIn = token.ExpiresIn
			p.AccessToken = token.AccessToken

			return p, nil
		} else if authn.Status == "MFA_ENROLL" {
			fmt.Println(string(c.ColorFailure), "Specified Okta organization requires MFA enrollment")
			fmt.Println(string(c.ColorFailure), "Configure your MFA device in Okta and try again")
			os.Exit(0)
		} else {
			c.Logger.Debug("Something went very wrong...")
			c.Logger.Debug("Status:", "status", authn.Status)
			os.Exit(0)
		}
	}

	return p, nil
}

func primaryAuth(c config.Configuration) (*authnResponse, error) {
	uri := c.Profile.OktaOrg + "/api/v1/authn"

	r := new(authnResponse)

	payload := map[string]interface{}{
		"username": c.Profile.Username,
		"password": c.Profile.Password,
		"options": map[string]interface{}{
			"multiOptionalFactorEnroll": true,
			"warnBeforePasswordExpired": false,
		},
	}
	mPayload, _ := json.Marshal(payload)
	contentReader := bytes.NewReader(mPayload)
	req, _ := http.NewRequest("POST", uri, contentReader)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Content-Type", "application/json")

	h := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := h.Do(req)
	if err != nil {
		fmt.Println(string(c.ColorFailure), "Unknown error: is the network up?")
		c.Logger.Debug("HTTP request failed", "error", err)
		os.Exit(0)
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		fmt.Println(string(c.ColorFailure), "Invalid password!")
		os.Exit(0)
	} else if resp.StatusCode != http.StatusOK {
		c.Logger.Debug("Primary: HTTP is not OK", "status", resp.StatusCode, "error", err)
		os.Exit(0)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		c.Logger.Debug("Unable to read response body", "error", err)
		os.Exit(0)
	}

	err = json.Unmarshal(body, &r)
	if err != nil {
		c.Logger.Debug("Unable to unmarshal response body", "error", err)
		os.Exit(0)
	}

	return r, nil
}

func verifyMFA(c config.Configuration, authn *authnResponse, factor *factor, challenge string) (*verifyResponse, error) {
	uri := factor.Links.Verify.VerifyURL

	r := new(verifyResponse)

	payload := map[string]interface{}{
		"stateToken": authn.StateToken,
		"passCode":   challenge,
	}
	mPayload, _ := json.Marshal(payload)
	contentReader := bytes.NewReader(mPayload)
	req, _ := http.NewRequest("POST", uri, contentReader)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Content-Type", "application/json")

	h := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := h.Do(req)
	if err != nil {
		c.Logger.Debug("HTTP request failed", "error", err)
		os.Exit(0)
	}
	if resp.StatusCode == http.StatusForbidden {
		fmt.Println(string(c.ColorFailure), "Invalid challenge!")
		os.Exit(0)
	} else if resp.StatusCode != http.StatusOK {
		c.Logger.Debug("Verify: HTTP is not OK", "status", resp.StatusCode, "error", err)
		os.Exit(0)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		c.Logger.Debug("Unable to read response body", "error", err)
		os.Exit(0)
	}

	err = json.Unmarshal(body, &r)
	if err != nil {
		c.Logger.Debug("Unable to unmarshal response body", "error", err)
		os.Exit(0)
	}

	return r, nil
}

func authCode(c config.Configuration, verify *verifyResponse) (*authorizeResponse, error) {
	uri := c.Profile.IssuerURL + "/v1/authorize"

	r := new(authorizeResponse)
	scope := "session:role-any"

	// PKCE code verifier and code challenge generation
	v, err := verifier.CreateCodeVerifier()
	if err != nil {
		return nil, err
	}
	r.CodeVerifier = v.String()
	codeChallenge := v.CodeChallengeS256()

	payload := url.Values{}
	payload.Set("client_id", c.Profile.ClientID)
	payload.Set("response_type", "code")
	payload.Set("scope", scope)
	payload.Set("redirect_uri", c.Profile.RedirectURI)
	payload.Set("state", uuid.NewString())
	payload.Set("sessionToken", verify.SessionToken)
	payload.Set("code_challenge", codeChallenge)
	payload.Set("code_challenge_method", "S256")

	req, _ := http.NewRequest("GET", uri, nil)
	req.URL.RawQuery = payload.Encode()

	h := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := h.Do(req)
	if err != nil {
		c.Logger.Debug("HTTP request failed", "error", err)
		os.Exit(0)
	}
	if resp.StatusCode != http.StatusFound {
		c.Logger.Debug("Authorize: HTTP is not 302!", "status", resp.StatusCode, "error", err)
		os.Exit(0)
	}

	location, err := resp.Location()
	if err != nil {
		c.Logger.Debug("Unable to read response location", "error", err)
		os.Exit(0)
	}

	r.State = location.Query().Get("state")
	r.Code = location.Query().Get("code")

	return r, nil
}

func oauthToken(c config.Configuration, auth *authorizeResponse) (*tokenResponse, error) {
	uri := c.Profile.IssuerURL + "/v1/token"

	r := new(tokenResponse)
	scope := "session:role-any"

	payload := url.Values{}

	if auth != nil {
		payload.Set("client_id", c.Profile.ClientID)
		payload.Set("grant_type", "authorization_code")
		payload.Set("code", auth.Code)
		payload.Set("code_verifier", auth.CodeVerifier)
		payload.Set("redirect_uri", c.Profile.RedirectURI)
		payload.Set("scope", scope)
	} else {
		payload.Set("client_id", c.Profile.ClientID)
		payload.Set("grant_type", "password")
		payload.Set("username", c.Profile.Username)
		payload.Set("password", c.Profile.Password)
		payload.Set("scope", scope)
	}

	req, _ := http.NewRequest("POST", uri, strings.NewReader(payload.Encode()))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")

	h := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := h.Do(req)
	if err != nil {
		c.Logger.Debug("HTTP request failed", "error", err, "response", resp)
		os.Exit(0)
	}
	if resp.StatusCode == http.StatusBadRequest {
		fmt.Println(string(c.ColorFailure), "Bad request: maybe check Okta privileges?")
		c.Logger.Debug("OAuth: HTTP 400", "error", err, "response", resp)
		os.Exit(0)
	} else if resp.StatusCode != http.StatusOK {
		c.Logger.Debug("OAuth: HTTP is not OK", "status", resp.StatusCode, "error", err)
		os.Exit(0)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		c.Logger.Debug("Unable to read response body", "error", err)
		os.Exit(0)
	}

	err = json.Unmarshal(body, &r)
	if err != nil {
		c.Logger.Debug("Unable to unmarshal response body", "error", err)
		os.Exit(0)
	}

	return r, nil
}

func factorPush(c config.Configuration, authn *authnResponse, factor *factor) error {
	uri := factor.Links.Verify.VerifyURL

	payload := map[string]interface{}{
		"stateToken": authn.StateToken,
	}
	mPayload, _ := json.Marshal(payload)
	contentReader := bytes.NewReader(mPayload)
	req, _ := http.NewRequest("POST", uri, contentReader)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	h := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := h.Do(req)
	if err != nil {
		c.Logger.Debug("HTTP request failed", "error", err)
		os.Exit(0)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		fmt.Println(string(c.ColorFailure), "Slow down! Wait a few moments...")
		os.Exit(0)
	} else if resp.StatusCode != http.StatusOK {
		c.Logger.Debug("Push: HTTP is not OK", "status", resp.StatusCode, "error", err)
		os.Exit(0)
	}

	if typeFactor("challenge", factor.FactorType) {
		fmt.Println(string(c.ColorSuccess), "MFA challenge sent!")
	}

	return nil
}

func retrievePassword(c config.Configuration) (config.Configuration, error) {
	store := func(password string) error {
		err := keyring.Set("gimme-snowflake-creds", c.Profile.Username, password)
		if err != nil {
			return err
		}

		fmt.Println(string(c.ColorSuccess), "Password saved to keyring")

		return nil
	}

	prompt := func() (string, error) {
		passwordLabel := "Okta password for " + c.Profile.Username
		keyringLabel := "Save this password in the keyring?"

		validate := func(input string) error {
			if len(input) == 0 {
				return errors.New("password must not be empty")
			}

			return nil
		}

		passwordPrompt := promptui.Prompt{
			Label:    passwordLabel,
			Validate: validate,
			Mask:     '*',
		}

		keyringPrompt := promptui.Prompt{
			Label:     keyringLabel,
			IsConfirm: true,
		}

		password, err := passwordPrompt.Run()
		if err != nil {
			return "", err
		}

		keyring, err := keyringPrompt.Run()
		if err != nil {
			return "", err
		}

		if keyring == "y" {
			store(password)
		}

		return password, nil
	}

	password, err := keyring.Get("gimme-snowflake-creds", c.Profile.Username)
	if err != nil {
		c.Logger.Debug("Password not present in keyring")
	}

	if password == "" {
		password, err := prompt()
		if err != nil {
			c.Logger.Debug("Prompt failed", "error", err)
			os.Exit(0)
		}

		c.Profile.Password = password

		return c, nil
	}

	c.Logger.Debug("Password present in keyring")
	c.Profile.Password = password

	return c, nil
}

func factorSelect(c config.Configuration, resp *authnResponse) (*factor, error) {
	factors := []string{}

	var r = new(factor)

	for _, f := range resp.Embedded.Factors {
		factors = append(factors, f.FactorType+" ("+f.Provider+")")
	}

	prompt := promptui.Select{
		Label: "Select MFA method",
		Items: factors,
	}

	_, result, err := prompt.Run()
	if err != nil {
		c.Logger.Debug("Prompt failed", "error", err)
		os.Exit(0)
	}

	for _, f := range resp.Embedded.Factors {
		if f.FactorType+" ("+f.Provider+")" == result {
			r = &f
			return r, nil
		}
	}

	return nil, nil
}

func factorChallenge(c config.Configuration, factor *factor) (string, error) {
	if typeFactor("activate", factor.FactorType) {
		return "", nil
	}

	validate := func(input string) error {
		if len(input) == 0 {
			return errors.New("MFA code must not be empty")
		}
		return nil
	}

	prompt := promptui.Prompt{
		Label:    "MFA code",
		Validate: validate,
		Mask:     '*',
	}

	result, err := prompt.Run()
	if err != nil {
		c.Logger.Debug("Prompt failed", "error", err)
		os.Exit(0)
	}

	return result, nil
}

func typeFactor(factorType string, factor string) bool {
	activateFactors := []string{"push"}
	challengeFactors := []string{"push", "sms"}

	if factorType == "activate" {
		for _, item := range activateFactors {
			if item == factor {
				return true
			}
		}
	} else if factorType == "challenge" {
		for _, item := range challengeFactors {
			if item == factor {
				return true
			}
		}
	}

	return false
}
