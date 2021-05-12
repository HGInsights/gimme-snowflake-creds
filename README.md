# gimme-snowflake-creds
[![build](https://github.com/HGInsights/gimme-snowflake-creds/actions/workflows/main.yml/badge.svg)](https://github.com/HGInsights/gimme-snowflake-creds/actions/workflows/main.yml)

CLI utility for retrieving ephemeral OAuth tokens for Snowflake via Okta.

Configuration and resulting OAuth tokens are used to generate profiled configurations for:
- [Snowflake ODBC connections](https://docs.snowflake.com/en/user-guide/odbc-parameters.html#odbc-configuration-and-connection-parameters)
- [DBT](https://docs.getdbt.com/docs/introduction)

Inspired by [gimme-aws-creds](https://github.com/Nike-Inc/gimme-aws-creds).

## Prerequisites

- [Okta / Snowflake OAuth integration](https://docs.snowflake.com/en/user-guide/oauth-okta.html#configure-okta-for-external-oauth)
- [Snowflake ODBC driver](https://docs.snowflake.com/en/user-guide/odbc.html)
- [DBT](https://docs.getdbt.com/dbt-cli/installation/)

## Installation
Download a [release](https://github.com/HGInsights/gimme-snowflake-creds/releases) and extract the contents:
```shell
tar -xvf gimme-snowflake-creds_0.1.0_darwin_arm64.tar.gz
```

Print a colon-separated list of locations in your `PATH`:
```shell
echo $PATH
```

Move the gimme-snowflake-creds binary to one of the locations listed in the previous step:
```shell
# Assumes you downloaded and extracted the binary in your `~/Downloads` directory
mv ~/Downloads/gimme-snowflake/creds /usr/local/bin/
```

## Configuration
`~/.okta_snowflake_login_config`
```yaml
dev: 
  account: <snowflake_account_id>
  database: <snowflake_database_name>
  warehouse: <snowflake_warehouse_name>
  role: <snowflake_role>
  username: <okta_username>
  okta-org: <okta_org_url>
  client-id: <okta_app_client_id>
  issuer-url: <okta_app_issuer_url>
  redirect-uri: <okta_app_redirect-uri>
  odbc-path: <path_to_odbc_ini_dir>  # Must be absolute path
  odbc-driver: <path_to_odbc_driver> # Must be absolute path
  
prod: 
  account: <snowflake_account_id>
  database: <snowflake_database_name>
  warehouse: <snowflake_warehouse_name>
  role: <snowflake_role>
  username: <okta_username>
  okta-org: <okta_org_url>
  client-id: <okta_app_client_id>
  issuer-url: <okta_app_issuer_url>
  redirect-uri: <okta_app_redirect-uri>
  odbc-path: <path_to_odbc_ini_dir>  # Must be absolute path
  odbc-driver: <path_to_odbc_driver> # Must be absolute path
```

## Usage
```shell
gimme-snowflake-creds -p dev
Okta password for gimme-user@example.com: ************************
✔ token:software:totp
MFA code: ******
 ODBC: Configuration written to: /Users/gimme.user/Library/ODBC/odbc.ini
 DBT: Configuration written to: /Users/gimme.user/.dbt/profiles.yml
```