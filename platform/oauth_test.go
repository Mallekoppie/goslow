package platform

import (
	"errors"
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"testing"
	"time"
)

func TestGetOAuth2Token(t *testing.T) {
	token, err := internalGetOAuth2Token("default")
	if err != nil {
		t.FailNow()
	}

	fmt.Println(token)
}

func TestGetTokenProperties(t *testing.T) {
	data := "eyJhbGciOiJSUzI1NiIsInR5cCIgOiAiSldUIiwia2lkIiA6ICJhZ1Nib3lKaHNnbnFtQ2g4VXFnOWtBajdCZ0xNSnpMdnNjS211OFpYSG5NIn0.eyJleHAiOjE2Mzk5OTgyMzksImlhdCI6MTYzOTk5ODE3OSwianRpIjoiODAyODNkN2QtOGJkNi00ZTY0LWJmZGUtY2JjMTA3ZDkxOGY2IiwiaXNzIjoiaHR0cDovL2xvY2FsaG9zdDo4MDgwL2F1dGgvcmVhbG1zL21hc3RlciIsImF1ZCI6ImFjY291bnQiLCJzdWIiOiI1NjNlNjA1Mi1lNDg0LTQxMDgtOTA5ZC1jYzQzYmM4ZTc0OWUiLCJ0eXAiOiJCZWFyZXIiLCJhenAiOiJmZWRlcmF0ZS10b2tlbi1jbGllbnQiLCJzZXNzaW9uX3N0YXRlIjoiNDdhMTk2NzEtZGMzNS00NDUxLWI1ZTktODAxNTlkNGQzN2E5IiwiYWNyIjoiMSIsImFsbG93ZWQtb3JpZ2lucyI6WyJodHRwOi8vbG9jYWxob3N0OjEwMTAwIl0sInJlYWxtX2FjY2VzcyI6eyJyb2xlcyI6WyJkZWZhdWx0LXJvbGVzLW1hc3RlciIsIm9mZmxpbmVfYWNjZXNzIiwidW1hX2F1dGhvcml6YXRpb24iXX0sInJlc291cmNlX2FjY2VzcyI6eyJhY2NvdW50Ijp7InJvbGVzIjpbIm1hbmFnZS1hY2NvdW50IiwibWFuYWdlLWFjY291bnQtbGlua3MiLCJ2aWV3LXByb2ZpbGUiXX19LCJzY29wZSI6InByb2ZpbGUgZW1haWwiLCJzaWQiOiI0N2ExOTY3MS1kYzM1LTQ0NTEtYjVlOS04MDE1OWQ0ZDM3YTkiLCJlbWFpbF92ZXJpZmllZCI6ZmFsc2UsInJvbGVzIjpbInVzZXIiXSwicHJlZmVycmVkX3VzZXJuYW1lIjoidGVzdCJ9.cfTkTZez2DcxiktTWWUGxtKKe_DBqgi1hIdsVu8vTz0kHs2DTeatCEDkmqQWxfzT4AWXQHyeoG8pXG1jBPH67B5QpdibYiCfb09NZL_x5PixG24PqlwBFD3ZD6BmLDM53QkIYEYDoYOhB60Ix02JiRS40byeMG-ofZGzU9Cf8fS-s3W-TkPkOFkU-6Yf7FvA2cczTfat_L07z1sQd8oNpp0O1FTuLgm_nUqskR5WYKLRFdSWaeWJYJ1YfLquea_mN4KXcEZIBdUcqPyhVpKTmifbr94YCjENBFBZpW6sQ5ukFS0P5Y-vlqqhb4aAqKjd8c9dcoCFYuAvX0wBlE8nMQ"

	token, err := jwt.Parse(data, func(token *jwt.Token) (interface{}, error) {
		return token, nil
	})
	if err != nil && errors.Is(err, jwt.ValidationError{}) {
		t.FailNow()
	}

	bla := token.Claims.(jwt.MapClaims)

	fmt.Println(bla)
	exp := bla["exp"].(float64)
	result := time.Unix(int64(exp), 0)

	fmt.Println(result)

	difference := time.Since(result)

	fmt.Println(difference)

	if difference.Minutes() > -5 {
		fmt.Println("Token must be renewed")
	} else {
		fmt.Println("Token must not be renewed")
	}
}

func TestValidateClientToken(t *testing.T) {
	rawToken := "eyJhbGciOiJSUzI1NiIsInR5cCIgOiAiSldUIiwia2lkIiA6ICJhZ1Nib3lKaHNnbnFtQ2g4VXFnOWtBajdCZ0xNSnpMdnNjS211OFpYSG5NIn0.eyJleHAiOjE2NDAwNzE1NjIsImlhdCI6MTY0MDA3MDk2MiwianRpIjoiZTVmNWY2MDEtMTc5ZS00MmM1LWJkNjAtOTFjOWFmNjUwMWMzIiwiaXNzIjoiaHR0cDovL2xvY2FsaG9zdDo4MDgwL2F1dGgvcmVhbG1zL21hc3RlciIsImF1ZCI6WyJmZWRlcmF0ZS10b2tlbi1jbGllbnQiLCJhY2NvdW50Il0sInN1YiI6IjU2M2U2MDUyLWU0ODQtNDEwOC05MDlkLWNjNDNiYzhlNzQ5ZSIsInR5cCI6IkJlYXJlciIsImF6cCI6ImZlZGVyYXRlLXRva2VuLWNsaWVudCIsInNlc3Npb25fc3RhdGUiOiJiMjQ2ZjE0Yy1iMTA4LTQzODYtOThjNi1mZGIwZGU5Y2ZmNTciLCJhY3IiOiIxIiwiYWxsb3dlZC1vcmlnaW5zIjpbImh0dHA6Ly9sb2NhbGhvc3Q6MTAxMDAiXSwicmVhbG1fYWNjZXNzIjp7InJvbGVzIjpbImRlZmF1bHQtcm9sZXMtbWFzdGVyIiwib2ZmbGluZV9hY2Nlc3MiLCJ1bWFfYXV0aG9yaXphdGlvbiJdfSwicmVzb3VyY2VfYWNjZXNzIjp7ImFjY291bnQiOnsicm9sZXMiOlsibWFuYWdlLWFjY291bnQiLCJtYW5hZ2UtYWNjb3VudC1saW5rcyIsInZpZXctcHJvZmlsZSJdfX0sInNjb3BlIjoicHJvZmlsZSBlbWFpbCIsInNpZCI6ImIyNDZmMTRjLWIxMDgtNDM4Ni05OGM2LWZkYjBkZTljZmY1NyIsImVtYWlsX3ZlcmlmaWVkIjpmYWxzZSwicm9sZXMiOlsidXNlciJdLCJwcmVmZXJyZWRfdXNlcm5hbWUiOiJ0ZXN0In0.KedzbxJdYiUGXRbwF_ucHGbos0cWCfE-fcK4Zm_OkZ85cEimcAG1qqdn7gPS_hNhIRUEqDgs_7NuCvdzjWY95amsARVyW_nvPCSiLVwc_c-aI2OEzpXSzIjlhTf5QDjSicovc-zwJMwpcOuzeBl7-gLj1riBKRPqgSjMLr1amNzPcCK9SL9MCaXxxjzLO9EpVLdqN2XvM8l9xdgJ5GfNDVnUhtSjIC9_ry7MSb4lfYxGdE1CwB2GcMicBIkoINp74KdGGTge3xbSHMBO0BbmqROKTSz8Z38EHb_OV2DiXczQFgqlJZN3Ux9sTFSag-0i5xVX8cv9zQFqcUy6f7A3Kg"

	parsedToken, err := ValidateClientToken(rawToken)
	if err != nil {
		t.FailNow()
	}

	fmt.Println(parsedToken)
	//token, _ := jwt.Parse(rawToken, func(token *jwt.Token) (interface{}, error) {
	//	return token, nil
	//})
	//
	//claims := token.Claims.(jwt.MapClaims)
	//issuer := claims["iss"].(string)
	//
	//jwksUrl, err := getJksUrl(issuer)
	//if err != nil {
	//	t.FailNow()
	//}
	//
	//parsedToken, err := jwt.Parse(rawToken, func(token *jwt.Token) (interface{}, error) {
	//	set, err := jwk.FetchHTTP(jwksUrl)
	//	if err != nil {
	//		return nil, err
	//	}
	//
	//	keyID, ok := token.Header["kid"].(string)
	//	if !ok {
	//		return nil, errors.New("expecting JWT header to have string kid")
	//	}
	//
	//	if key := set.LookupKeyID(keyID); len(key) == 1 {
	//		return key[0].Materialize()
	//	}
	//
	//	return nil, fmt.Errorf("unable to find key %q", keyID)
	//})
	//if err != nil {
	//	fmt.Println("Token Validation Failed")
	//	t.FailNow()
	//}
	//
	//fmt.Println(parsedToken)

}
