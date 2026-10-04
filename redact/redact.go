package redact

import (
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var userinfo = regexp.MustCompile(`(://)[^/\s"']*@`)

const maskedUserinfo = "${1}xxxxx@"

var secretQueryParams = map[string]struct{}{
	"access_token": {},
	"webapi_token": {},
	"oauth_token":  {},
	"key":          {},
	"api_key":      {},
	"token":        {},
}

const maskedSecret = "xxxxx"

type redactedError struct {
	text string
	err  error
}

func (e redactedError) Error() string { return e.text }

func (e redactedError) Unwrap() error { return e.err }

func Error(err error) error {
	if err == nil {
		return nil
	}

	text := err.Error()

	var urlErr *url.Error

	carriesURL := errors.As(err, &urlErr)
	if carriesURL {
		text = keepOrigins(text, urlErr)
	}

	masked := userinfo.ReplaceAllString(text, maskedUserinfo)
	if !carriesURL && masked == text {
		return err
	}

	return redactedError{text: masked, err: err}
}

func keepOrigins(text string, urlErr *url.Error) string {
	for cause := error(urlErr); errors.As(cause, &urlErr); cause = urlErr.Err {
		if urlErr.URL != "" {
			kept := origin(urlErr.URL)
			text = strings.ReplaceAll(text, strconv.Quote(urlErr.URL), strconv.Quote(kept))
			text = strings.ReplaceAll(text, urlErr.URL, kept)
		}
	}

	return text
}

func origin(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}

	return parsed.Scheme + "://" + parsed.Host
}

func Args(args []any) []any {
	redacted := make([]any, len(args))
	for i, arg := range args {
		redacted[i] = Value(arg)
	}

	return redacted
}

func Value(value any) any {
	switch typed := value.(type) {
	case error:
		return Error(typed)
	case string:
		return maskURL(typed)
	}

	return value
}

func maskURL(text string) string {
	link, err := url.Parse(text)
	if err != nil || link.Host == "" || link.RawQuery == "" {
		return text
	}

	query := link.Query()
	for name := range query {
		if _, secret := secretQueryParams[name]; secret {
			query.Set(name, maskedSecret)
		}
	}

	link.RawQuery = query.Encode()

	return link.Redacted()
}
