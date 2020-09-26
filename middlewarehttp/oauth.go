package middleware

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/coreos/go-oidc"
)

type CustomClaims struct {
	Roles []string `json:"roles,omitempty"`
}

func UseOAuth2(inner http.Handler, roles []string) http.Handler {

	ctx := context.Background()
	provider, err := oidc.NewProvider(ctx, "http://localhost:8180/auth/realms/golang") // this is bad
	if err != nil {
		panic(err)
	}

	oidcConfig := &oidc.Config{
		ClientID: "gotutorial", // this is bad
	}
	verifier := provider.Verifier(oidcConfig)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		authorized := true

		// Only check the claims if a specific role is required for the path
		//if 1 == 0 {
		if len(roles) > 0 {
			authorized = false
			rawAccessToken := r.Header.Get("Authorization")

			if rawAccessToken == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			parts := strings.Split(rawAccessToken, " ")
			if len(parts) != 2 {
				log.Println("Auth header not build correctly")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			idToken, err := verifier.Verify(ctx, parts[1])
			if err != nil {
				log.Println("Token verification failed: ", err.Error())
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			claims := CustomClaims{}
			err = idToken.Claims(&claims)
			if err != nil {
				log.Println("Unable to get claims: ", err.Error())
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			injectRoles := ""
			for neededlIndex := range roles {
				for tokenRoleIndex := range claims.Roles {
					if roles[neededlIndex] == claims.Roles[tokenRoleIndex] {
						authorized = true
						injectRoles = injectRoles + fmt.Sprintf(",%s", roles[neededlIndex])
					}
				}
			}

			if authorized != true {
				log.Println("Required role not found: ", roles)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			r.Header.Add("X-Token-Roles", injectRoles)
		}

		//start := time.Now()
		if authorized {
			inner.ServeHTTP(w, r)
		}

		r.Header.Del("X-Token-Roles")
	})
}
