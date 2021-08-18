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

func Auth(p config.Configuration) (*config.Credentials, error) {
	c := new(config.Credentials)

	if p.OAuth {
		// Prompt for Okta password
		p, err := passwordPrompt(p)
		if err != nil {
			p.Logger.Debug("Unable to return password prompt input", "error", err)
			os.Exit(0)
		}

		// Perform primary, initial authentication
		authn, err := primaryAuth(p)
		if err != nil {
			p.Logger.Debug("Unable to return primary authentication response", "error", err)
			os.Exit(0)
		}

		if authn.Status == "SUCCESS" {
			// Retrieve OAuth token
			token, err := oauthToken(p, nil)
			if err != nil {
				p.Logger.Debug("Unable to return OAuth token", "error", err)
				os.Exit(0)
			}

			c.ExpiresIn = token.ExpiresIn
			c.AccessToken = token.AccessToken

			return c, nil
		} else if authn.Status == "MFA_REQUIRED" {
			// Prompt for factor type
			factor, err := factorSelect(p, authn)
			if err != nil {
				p.Logger.Debug("Unable to return factor selection", "error", err)
				os.Exit(0)
			}

			err = factorPush(p, authn, factor)
			if err != nil {
				p.Logger.Debug("Unable to initiate factor push", "error", err)
				os.Exit(0)
			}

			// Prompt for factor challenge
			challenge, err := factorChallenge(p, factor)
			if err != nil {
				p.Logger.Debug("Unable to return factor challenge", "error", err)
				os.Exit(0)
			}

			// Perform MFA verification
			verify, err := verifyMFA(p, authn, factor, challenge)
			for verify.FactorResult == "WAITING" {
				p.Logger.Debug("Checking MFA verification...")
				time.Sleep(1 * time.Second)
				verify, err = verifyMFA(p, authn, factor, challenge)
			}
			if verify.FactorResult == "REJECTED" {
				fmt.Println(string(p.ColorFailure), "MFA challenge rejected!")
				os.Exit(0)
			} else if verify.FactorResult == "TIMEOUT" {
				fmt.Println(string(p.ColorSuccess), "MFA challenge timed out!")
				os.Exit(0)
			} else if verify.Status == "SUCCESS" {
				fmt.Println(string(p.ColorSuccess), "MFA verified!")
			}
			if err != nil {
				fmt.Println(string(p.ColorFailure), "MFA verification failed!")
				p.Logger.Debug("Unable to return MFA verifcation", "error", err)
				os.Exit(0)
			}

			// Retrieve authorizataion code
			auth, err := authCode(p, verify)
			if err != nil {
				p.Logger.Debug("Unable to return authorization code", "error", err)
				os.Exit(0)
			}

			// Retrieve OAuth token
			token, err := oauthToken(p, auth)
			if err != nil {
				p.Logger.Debug("Unable to return OAuth token", "error", err)
				os.Exit(0)
			}

			c.ExpiresIn = token.ExpiresIn
			c.AccessToken = token.AccessToken

			return c, nil
		} else {
			p.Logger.Debug("Something went very wrong...")
			os.Exit(0)
		}
	}

	return c, nil
}

func primaryAuth(p config.Configuration) (*authnResponse, error) {
	uri := p.OktaOrg + "/api/v1/authn"

	r := new(authnResponse)

	payload := map[string]interface{}{
		"username": p.Username,
		"password": p.Password,
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

	c := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := c.Do(req)
	if err != nil {
		fmt.Println(string(p.ColorFailure), "Unknown error: is the network up?")
		p.Logger.Debug("HTTP request failed", "error", err)
		os.Exit(0)
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		fmt.Println(string(p.ColorFailure), "Invalid password!")
		os.Exit(0)
	} else if resp.StatusCode != http.StatusOK {
		p.Logger.Debug("Primary: HTTP is not OK", "status", resp.StatusCode, "error", err)
		os.Exit(0)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		p.Logger.Debug("Unable to read response body", "error", err)
		os.Exit(0)
	}

	err = json.Unmarshal(body, &r)
	if err != nil {
		p.Logger.Debug("Unable to unmarshal response body", "error", err)
		os.Exit(0)
	}

	return r, nil
}

func verifyMFA(p config.Configuration, authn *authnResponse, factor *factor, challenge string) (*verifyResponse, error) {
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

	c := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := c.Do(req)
	if err != nil {
		p.Logger.Debug("HTTP request failed", "error", err)
		os.Exit(0)
	}
	if resp.StatusCode == http.StatusForbidden {
		fmt.Println(string(p.ColorFailure), "Invalid challenge!")
		os.Exit(0)
	} else if resp.StatusCode != http.StatusOK {
		p.Logger.Debug("Verify: HTTP is not OK", "status", resp.StatusCode, "error", err)
		os.Exit(0)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		p.Logger.Debug("Unable to read response body", "error", err)
		os.Exit(0)
	}

	err = json.Unmarshal(body, &r)
	if err != nil {
		p.Logger.Debug("Unable to unmarshal response body", "error", err)
		os.Exit(0)
	}

	return r, nil
}

func authCode(p config.Configuration, verify *verifyResponse) (*authorizeResponse, error) {
	uri := p.IssuerURL + "/v1/authorize"

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
	payload.Set("client_id", p.ClientID)
	payload.Set("response_type", "code")
	payload.Set("scope", scope)
	payload.Set("redirect_uri", p.RedirectURI)
	payload.Set("state", uuid.NewString())
	payload.Set("sessionToken", verify.SessionToken)
	payload.Set("code_challenge", codeChallenge)
	payload.Set("code_challenge_method", "S256")

	req, _ := http.NewRequest("GET", uri, nil)
	req.URL.RawQuery = payload.Encode()

	c := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := c.Do(req)
	if err != nil {
		p.Logger.Debug("HTTP request failed", "error", err)
		os.Exit(0)
	}
	if resp.StatusCode != http.StatusFound {
		p.Logger.Debug("Authorize: HTTP is not 302!", "status", resp.StatusCode, "error", err)
		os.Exit(0)
	}

	location, err := resp.Location()
	if err != nil {
		p.Logger.Debug("Unable to read response location", "error", err)
		os.Exit(0)
	}

	r.State = location.Query().Get("state")
	r.Code = location.Query().Get("code")

	return r, nil
}

func oauthToken(p config.Configuration, auth *authorizeResponse) (*tokenResponse, error) {
	uri := p.IssuerURL + "/v1/token"

	r := new(tokenResponse)
	scope := "session:role-any"

	payload := url.Values{}

	if auth != nil {
		payload.Set("client_id", p.ClientID)
		payload.Set("grant_type", "authorization_code")
		payload.Set("code", auth.Code)
		payload.Set("code_verifier", auth.CodeVerifier)
		payload.Set("redirect_uri", p.RedirectURI)
		payload.Set("scope", scope)
	} else {
		payload.Set("client_id", p.ClientID)
		payload.Set("grant_type", "password")
		payload.Set("username", p.Username)
		payload.Set("password", p.Password)
		payload.Set("scope", scope)
	}

	req, _ := http.NewRequest("POST", uri, strings.NewReader(payload.Encode()))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")

	c := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := c.Do(req)
	if err != nil {
		p.Logger.Debug("HTTP request failed", "error", err, "response", resp)
		os.Exit(0)
	}
	if resp.StatusCode == http.StatusBadRequest {
		fmt.Println(string(p.ColorFailure), "Bad request: maybe check Okta privileges?")
		p.Logger.Debug("OAuth: HTTP 400", "error", err, "response", resp)
		os.Exit(0)
	} else if resp.StatusCode != http.StatusOK {
		p.Logger.Debug("OAuth: HTTP is not OK", "status", resp.StatusCode, "error", err)
		os.Exit(0)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		p.Logger.Debug("Unable to read response body", "error", err)
		os.Exit(0)
	}

	err = json.Unmarshal(body, &r)
	if err != nil {
		p.Logger.Debug("Unable to unmarshal response body", "error", err)
		os.Exit(0)
	}

	return r, nil
}

func factorPush(p config.Configuration, authn *authnResponse, factor *factor) error {
	uri := factor.Links.Verify.VerifyURL

	payload := map[string]interface{}{
		"stateToken": authn.StateToken,
	}
	mPayload, _ := json.Marshal(payload)
	contentReader := bytes.NewReader(mPayload)
	req, _ := http.NewRequest("POST", uri, contentReader)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	c := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := c.Do(req)
	if err != nil {
		p.Logger.Debug("HTTP request failed", "error", err)
		os.Exit(0)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		fmt.Println(string(p.ColorFailure), "Slow down! Wait a few moments...")
		os.Exit(0)
	} else if resp.StatusCode != http.StatusOK {
		p.Logger.Debug("Push: HTTP is not OK", "status", resp.StatusCode, "error", err)
		os.Exit(0)
	}

	if typeFactor("challenge", factor.FactorType) {
		fmt.Println(string(p.ColorSuccess), "MFA challenge sent!")
	}

	return nil
}

func passwordPrompt(p config.Configuration) (config.Configuration, error) {
	label := "Okta password for " + p.Username

	validate := func(input string) error {
		if len(input) == 0 {
			return errors.New("password must not be empty")
		}
		return nil
	}

	prompt := promptui.Prompt{
		Label:    label,
		Validate: validate,
		Mask:     '*',
	}

	result, err := prompt.Run()
	if err != nil {
		p.Logger.Debug("Prompt failed", "error", err)
		os.Exit(0)
	}

	p.Password = result

	return p, nil
}

func factorSelect(p config.Configuration, resp *authnResponse) (*factor, error) {
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
		p.Logger.Debug("Prompt failed", "error", err)
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

func factorChallenge(p config.Configuration, factor *factor) (string, error) {
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
		p.Logger.Debug("Prompt failed", "error", err)
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
