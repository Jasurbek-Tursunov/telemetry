package redact_test

import (
	"errors"
	"fmt"
	"maps"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/Jasurbek-Tursunov/telemetry/redact"
)

const (
	moneySendURL      = "http://market.test/money-send/100/SECRETKEY?key=SECRET&pay_pass=SECRET2"
	marketOrigin      = "http://market.test"
	redactedMoneySend = `Get "` + marketOrigin + `": connection dropped`
	secretMarker      = "SECRET"
)

var errDropped = errors.New("connection dropped")

func failedGet(rawURL string, cause error) *url.Error {
	return &url.Error{Op: "Get", URL: rawURL, Err: cause}
}

func TestRedactKeepsOnlyTheOrigin(t *testing.T) {
	if got := redact.Error(failedGet(moneySendURL, errDropped)).Error(); got != redactedMoneySend {
		t.Fatalf("redacted = %q, want %q", got, redactedMoneySend)
	}
}

func TestRedactCleansNestedURLs(t *testing.T) {
	err := failedGet("http://market.test/redirect?token=SECRET",
		failedGet("http://steam.test/login?session=SECRET2", errDropped))

	want := `Get "` + marketOrigin + `": Get "http://steam.test": connection dropped`
	if got := redact.Error(err).Error(); got != want {
		t.Fatalf("redacted = %q, want %q", got, want)
	}
}

func TestRedactCleansAURLThatDoesNotParse(t *testing.T) {
	recipient := "KEY\nX"

	_, err := url.Parse("https://market.test/money-send/1/" + recipient + "?key=" + secretMarker)
	if err == nil {
		t.Fatal("expected a URL with a control character to be refused")
	}

	redacted := redact.Error(err).Error()

	if strings.Contains(redacted, "KEY") || strings.Contains(redacted, secretMarker) {
		t.Fatalf("redacted = %q, still carries the recipient key or the secret", redacted)
	}
}

func TestRedactKeepsTheChain(t *testing.T) {
	err := failedGet(moneySendURL, fmt.Errorf("proxyconnect: %w", syscall.ECONNREFUSED))

	redacted := redact.Error(err)

	if strings.Contains(redacted.Error(), secretMarker) {
		t.Fatalf("redacted = %q, still carries a secret", redacted)
	}

	if !errors.Is(redacted, syscall.ECONNREFUSED) {
		t.Fatalf("redacted = %v, lost ECONNREFUSED", redacted)
	}

	var urlErr *url.Error
	if !errors.As(redacted, &urlErr) {
		t.Fatalf("redacted = %v, lost the url.Error", redacted)
	}
}

func TestRedactLeavesErrorsWithoutURLAlone(t *testing.T) {
	redacted := redact.Error(nil)
	if redacted != nil {
		t.Fatalf("Redact(nil) = %v, want nil", redacted)
	}

	plain := fmt.Errorf("balance refused: %w", errDropped)

	redacted = redact.Error(plain)
	if any(redacted) != any(plain) {
		t.Fatalf("Redact(%v) = %v, want the same error back", plain, redacted)
	}
}

const (
	proxyUser         = "proxyuser"
	proxyPassword     = "s3cr3t"
	proxyWithPassword = "http://" + proxyUser + ":" + proxyPassword + "@10.0.0.5:3128"
	maskedProxy       = "http://xxxxx@10.0.0.5:3128"
)

func TestRedactMasksTheCredentialsOfAURLInTheText(t *testing.T) {
	err := fmt.Errorf("proxyconnect: %s refused: %w", proxyWithPassword, errDropped)

	redacted := redact.Error(err)

	if want := "proxyconnect: " + maskedProxy + " refused: connection dropped"; redacted.Error() != want {
		t.Fatalf("redacted = %q, want %q", redacted, want)
	}

	if !errors.Is(redacted, errDropped) {
		t.Errorf("redacted = %v, lost the chain", redacted)
	}
}

func TestRedactMasksAPasswordThatHoldsAnAt(t *testing.T) {
	err := fmt.Errorf("dial http://%s:p@ss@10.0.0.5:3128/x: %w", proxyUser, errDropped)

	if want := "dial http://xxxxx@10.0.0.5:3128/x: connection dropped"; redact.Error(err).Error() != want {
		t.Fatalf("redacted = %q, want %q", redact.Error(err), want)
	}
}

func TestRedactMasksTheCredentialsOfAQuotedURLThatIsNoURLError(t *testing.T) {
	err := errors.New(`Get "` + proxyWithPassword + `/path": eof`)

	if want := `Get "` + maskedProxy + `/path": eof`; redact.Error(err).Error() != want {
		t.Fatalf("redacted = %q, want %q", redact.Error(err), want)
	}
}

func TestRedactMasksAProxyURLThatDoesNotParse(t *testing.T) {
	_, err := url.Parse(strings.Replace(proxyWithPassword, ":3128", ":port", 1))
	if err == nil {
		t.Fatal("expected a proxy URL with a bad port to be refused")
	}

	redacted := redact.Error(err).Error()

	if strings.Contains(redacted, proxyPassword) || strings.Contains(redacted, proxyUser) {
		t.Fatalf("redacted = %q, still carries the proxy credentials", redacted)
	}
}

func TestRedactMasksTheCredentialsOfAURLNestedInAURLError(t *testing.T) {
	err := failedGet("https://steam.test/login?session="+secretMarker,
		fmt.Errorf("via %s: %w", proxyWithPassword, errDropped))

	redacted := redact.Error(err).Error()

	for _, secret := range []string{proxyPassword, proxyUser, secretMarker} {
		if strings.Contains(redacted, secret) {
			t.Fatalf("redacted = %q, still carries %q", redacted, secret)
		}
	}
}

func TestRedactLeavesAnAtThatIsNoCredentialAlone(t *testing.T) {
	for _, text := range []string{
		"mail owner@example.com refused",
		"Get http://market.test/a?email=a@b.c: eof",
		"dial tcp 10.0.0.5:3128: user@host",
	} {
		plain := errors.New(text)

		redacted := redact.Error(plain)
		if any(redacted) != any(plain) {
			t.Errorf("Redact(%q) = %q, want the same error back", text, redacted)
		}
	}
}

func TestRedactArgsCleansOnlyErrors(t *testing.T) {
	args := []any{"attempt", failedGet(moneySendURL, errDropped), 2}

	redacted := redact.Args(args)

	if len(redacted) != len(args) {
		t.Fatalf("len = %d, want %d", len(redacted), len(args))
	}

	if redacted[0] != args[0] || redacted[2] != args[2] {
		t.Fatalf("redacted = %v, want the non-error arguments untouched", redacted)
	}

	if text := fmt.Sprint(redacted[1]); text != redactedMoneySend {
		t.Fatalf("error argument = %q, still carries the request URL", text)
	}
}

const (
	standParentalURL = "https://api.steampowered.com/IParentalService/GetParentalSettings/v1" +
		"?access_token=eyJ.abc.def&origin=http%3A%2F%2Flocalhost%3A9090"
	standParentalRedacted = "https://api.steampowered.com/IParentalService/GetParentalSettings/v1" +
		"?access_token=xxxxx&origin=http%3A%2F%2Flocalhost%3A9090"
)

func TestRedactArgsMasksSecretQueryParamsOfAURLString(t *testing.T) {
	const endpoint = "https://api.test/a?"

	cases := map[string]struct {
		raw  string
		want string
	}{
		"the stand parental settings url": {raw: standParentalURL, want: standParentalRedacted},
		"a webapi token":                  {raw: endpoint + "webapi_token=SECRET", want: endpoint + "webapi_token=xxxxx"},
		"an oauth token":                  {raw: endpoint + "oauth_token=SECRET", want: endpoint + "oauth_token=xxxxx"},
		"an api key":                      {raw: endpoint + "api_key=SECRET", want: endpoint + "api_key=xxxxx"},
		"a bare token":                    {raw: endpoint + "token=SECRET", want: endpoint + "token=xxxxx"},
		"a repeated key":                  {raw: endpoint + "key=SECRET&key=SECRET2", want: endpoint + "key=xxxxx"},
		"a secret beside plain params": {
			raw:  endpoint + "count=2&key=SECRET&l=english",
			want: endpoint + "count=2&key=xxxxx&l=english",
		},
		"a password in the userinfo": {raw: "https://user:pass@host/?key=1", want: "https://user:xxxxx@host/?key=xxxxx"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			redacted := redact.Args([]any{tc.raw})

			if got := fmt.Sprint(redacted[0]); got != tc.want {
				t.Fatalf("redacted = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRedactArgsLosesTheUserinfoPasswordAndTheKey(t *testing.T) {
	redacted := fmt.Sprint(redact.Args([]any{"https://user:pass@host/?key=1"})[0])

	if strings.Contains(redacted, "pass@") || strings.Contains(redacted, "key=1") {
		t.Fatalf("redacted = %q, still carries the password or the key", redacted)
	}
}

func TestRedactArgsKeepsTheMeaningOfAURLWithoutSecrets(t *testing.T) {
	raw := "https://steamcommunity.com/inventory/76561198000000000/440/2?l=english&count=2000&start_assetid=42"

	redacted, isText := redact.Args([]any{raw})[0].(string)
	if !isText {
		t.Fatal("a string argument came back as another type")
	}

	want, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}

	got, err := url.Parse(redacted)
	if err != nil {
		t.Fatalf("redacted = %q, no longer parses: %v", redacted, err)
	}

	if got.Scheme != want.Scheme || got.Host != want.Host || got.Path != want.Path {
		t.Fatalf("redacted = %q, want the origin and path of %q", redacted, raw)
	}

	if !maps.EqualFunc(got.Query(), want.Query(), slices.Equal[[]string]) {
		t.Fatalf("redacted query = %v, want %v", got.Query(), want.Query())
	}
}

func TestRedactArgsLeavesStringsThatAreNoQueryURLAlone(t *testing.T) {
	for _, text := range []string{
		"service.steam.bot.Start",
		"supervisor.Supervisor.reconcileInBackground",
		"",
		"76561198000000000",
		"FETCH_STEAM_PAGE failed: key=1 was refused",
		"/inventory/76561198000000000/440/2",
		"/relative/path?key=SECRET",
		"https://steamcommunity.com/inventory/1/440/2",
		"https://steamcommunity.com/inventory/1/440/2?",
		"localhost:9090",
		"http://bad host/?key=SECRET",
	} {
		redacted := redact.Args([]any{text})

		if redacted[0] != text {
			t.Errorf("RedactArgs(%q) = %q, want the text back unchanged", text, redacted[0])
		}
	}
}

func TestRedactArgsLeavesNonStringArgumentsUntouched(t *testing.T) {
	type payload struct{ Token string }

	args := []any{
		42, 2.5, true, nil,
		[]string{standParentalURL}, payload{Token: "SECRET"}, map[string]string{"key": "SECRET"},
	}

	redacted := redact.Args(args)

	if !reflect.DeepEqual(redacted, args) {
		t.Fatalf("redacted = %v, want the non-string arguments untouched", redacted)
	}
}
