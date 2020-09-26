"platform" = {
  "LogLevel" = "info"

  "Http" = {
    "ListeningAddress" = "0.0.0.0:9111"

    "TlsCertFileName" = ""

    "TlsKeyFileName" = ""

    "TlsEnabled" = false
  }

  "Auth" = {
    "OAuthEnabled" = false

    "IdpWellKnownUrl" = ""

    "OwnTokens" = {
      "ClientId" = ""

      "ClientSecret" = ""
    }
  }

  "Component" = {
    "ComponentName" = "Unit Test"

    "ComponentConfigFileName" = "serviceconfigfile.hcl"
  }
}