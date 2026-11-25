package codearts

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

type DPoPJWK struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
	D   string `json:"d,omitempty"`
}

type PKCEPair struct {
	Verifier  string
	Challenge string
}

func GeneratePKCE() (PKCEPair, error) {
	raw := make([]byte, 48)
	if _, err := rand.Read(raw); err != nil {
		return PKCEPair{}, err
	}
	verifier := base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(verifier))
	return PKCEPair{
		Verifier:  verifier,
		Challenge: base64.RawURLEncoding.EncodeToString(sum[:]),
	}, nil
}

func GenerateDPoPKey() (DPoPJWK, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return DPoPJWK{}, err
	}
	return DPoPJWK{
		Kty: "EC",
		Crv: "P-256",
		X:   padCoord(priv.X),
		Y:   padCoord(priv.Y),
		D:   padCoord(priv.D),
	}, nil
}

func padCoord(val *big.Int) string {
	var buf [32]byte
	val.FillBytes(buf[:])
	return base64.RawURLEncoding.EncodeToString(buf[:])
}

func parseDPoPPrivateKey(jwk DPoPJWK) (*ecdsa.PrivateKey, DPoPJWK, error) {
	if jwk.Kty != "EC" || jwk.Crv != "P-256" || jwk.X == "" || jwk.Y == "" || jwk.D == "" {
		return nil, DPoPJWK{}, errors.New("invalid P-256 DPoP JWK")
	}
	xb, err := base64.RawURLEncoding.DecodeString(jwk.X)
	if err != nil {
		return nil, DPoPJWK{}, err
	}
	yb, err := base64.RawURLEncoding.DecodeString(jwk.Y)
	if err != nil {
		return nil, DPoPJWK{}, err
	}
	db, err := base64.RawURLEncoding.DecodeString(jwk.D)
	if err != nil {
		return nil, DPoPJWK{}, err
	}
	priv := &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{
			Curve: elliptic.P256(),
			X:     new(big.Int).SetBytes(xb),
			Y:     new(big.Int).SetBytes(yb),
		},
		D: new(big.Int).SetBytes(db),
	}
	pub := DPoPJWK{Kty: "EC", Crv: "P-256", X: jwk.X, Y: jwk.Y}
	return priv, pub, nil
}

func SignDPoPJWS(jwk DPoPJWK, htm, htu string, now time.Time) (string, error) {
	priv, pub, err := parseDPoPPrivateKey(jwk)
	if err != nil {
		return "", err
	}
	headerBytes, err := json.Marshal(map[string]any{
		"alg": "ES256",
		"typ": "dpop+jwt",
		"jwk": pub,
	})
	if err != nil {
		return "", err
	}
	payloadBytes, err := json.Marshal(map[string]any{
		"htm": htm,
		"htu": htu,
		"iat": now.Unix(),
		"jti": randomHex(32),
	})
	if err != nil {
		return "", err
	}
	signingInput := base64.RawURLEncoding.EncodeToString(headerBytes) + "." + base64.RawURLEncoding.EncodeToString(payloadBytes)
	digest := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, priv, digest[:])
	if err != nil {
		return "", err
	}
	var sig [64]byte
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig[:]), nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func hmacSHA256Bytes(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

func hmacSHA256Hex(key, data []byte) string {
	return hex.EncodeToString(hmacSHA256Bytes(key, data))
}

func rfc3986(input string) string {
	var b strings.Builder
	for i := 0; i < len(input); i++ {
		ch := input[i]
		if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') ||
			ch == '-' || ch == '_' || ch == '.' || ch == '~' {
			b.WriteByte(ch)
		} else {
			fmt.Fprintf(&b, "%%%02X", ch)
		}
	}
	return b.String()
}

func canonicalQuery(u *url.URL) string {
	type pair struct{ k, v string }
	q := u.Query()
	var pairs []pair
	for k, vals := range q {
		if len(vals) == 0 {
			pairs = append(pairs, pair{k, ""})
			continue
		}
		for _, v := range vals {
			pairs = append(pairs, pair{k, v})
		}
	}
	slices.SortFunc(pairs, func(a, b pair) int {
		if a.k != b.k {
			return strings.Compare(a.k, b.k)
		}
		return strings.Compare(a.v, b.v)
	})
	parts := make([]string, len(pairs))
	for idx, item := range pairs {
		parts[idx] = rfc3986(item.k) + "=" + rfc3986(item.v)
	}
	return strings.Join(parts, "&")
}

func sdkDateStamp(now time.Time) string {
	return now.UTC().Format("20060102T150405Z")
}

func SignRequestHuawei(ak, sk, securityToken, method, urlStr string, body []byte, extraHeaders map[string]string, dateOverride string) (map[string]string, error) {
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	uri := parsed.Path
	if uri == "" {
		uri = "/"
	}
	if !strings.HasSuffix(uri, "/") {
		uri += "/"
	}
	query := canonicalQuery(parsed)
	dateStamp := dateOverride
	if dateStamp == "" {
		dateStamp = sdkDateStamp(time.Now())
	}
	payloadHash := sha256Hex(body)
	methodUpper := strings.ToUpper(method)
	headers := map[string]string{
		"host":                 parsed.Host,
		"x-sdk-date":           dateStamp,
		"x-sdk-content-sha256": payloadHash,
		"x-security-token":     securityToken,
	}
	for k, v := range extraHeaders {
		headers[strings.ToLower(k)] = v
	}
	if methodUpper != http.MethodGet {
		if _, exists := headers["content-type"]; !exists {
			headers["content-type"] = "application/json"
		}
	}
	signedHeaders := make([]string, 0, len(headers))
	for k := range headers {
		signedHeaders = append(signedHeaders, k)
	}
	slices.Sort(signedHeaders)
	headerLines := make([]string, len(signedHeaders))
	for idx, k := range signedHeaders {
		headerLines[idx] = k + ":" + headers[k]
	}
	canonicalRequest := strings.Join([]string{
		methodUpper,
		uri,
		query,
		strings.Join(headerLines, "\n"),
		"",
		strings.Join(signedHeaders, ";"),
		payloadHash,
	}, "\n")
	canonicalHash := sha256Hex([]byte(canonicalRequest))
	stringToSign := "SDK-HMAC-SHA256\n" + dateStamp + "\n" + canonicalHash
	signature := hmacSHA256Hex([]byte(sk), []byte(stringToSign))
	headers["Authorization"] = fmt.Sprintf("SDK-HMAC-SHA256 Access=%s,SignedHeaders=%s,Signature=%s",
		ak, strings.Join(signedHeaders, ";"), signature)
	return headers, nil
}

func SignOfficeAceV11(urlStr string, ak, sk, securityToken, projectID, method string, body []byte, dateOverride string) (map[string]string, error) {
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	xdate := dateOverride
	if xdate == "" {
		xdate = sdkDateStamp(time.Now())
	}
	rawSegs := strings.Split(parsed.Path, "/")
	encSegs := make([]string, len(rawSegs))
	for idx, seg := range rawSegs {
		decoded, decErr := url.PathUnescape(seg)
		if decErr != nil {
			decoded = seg
		}
		encSegs[idx] = rfc3986(decoded)
	}
	canonicalURI := strings.Join(encSegs, "/")
	if !strings.HasSuffix(canonicalURI, "/") {
		canonicalURI += "/"
	}
	methodUpper := strings.ToUpper(method)
	original := map[string]string{
		"x-subscription-type": "v2",
		"X-Sdk-Date":          xdate,
		"X-Security-Token":    strings.TrimSpace(securityToken),
		"Host":                parsed.Host,
	}
	if methodUpper == http.MethodPost {
		original["Content-Type"] = "application/json;charset=utf8"
	}
	if strings.TrimSpace(projectID) != "" {
		original["X-Project-ID"] = strings.TrimSpace(projectID)
	}
	lower := make(map[string]string, len(original))
	signedHeaders := make([]string, 0, len(original))
	for k, v := range original {
		lk := strings.ToLower(k)
		lower[lk] = collapseSpaces(strings.TrimSpace(v))
		signedHeaders = append(signedHeaders, lk)
	}
	slices.Sort(signedHeaders)
	var canonHdrs strings.Builder
	for _, h := range signedHeaders {
		canonHdrs.WriteString(h)
		canonHdrs.WriteByte(':')
		canonHdrs.WriteString(lower[h])
		canonHdrs.WriteByte('\n')
	}
	payloadHash := sha256Hex(body)
	canonicalRequest := strings.Join([]string{
		methodUpper,
		canonicalURI,
		canonicalQuery(parsed),
		canonHdrs.String(),
		strings.Join(signedHeaders, ";"),
		payloadHash,
	}, "\n")
	scope := xdate[:8] + "/" + OARegion + "/" + OAService
	stringToSign := strings.Join([]string{
		"V11-HMAC-SHA256",
		xdate,
		scope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")
	u1 := hmacSHA256Bytes([]byte(ak), []byte(sk))
	scopePlusOne := append([]byte(scope), 1)
	derived := hmacSHA256Bytes(u1, scopePlusOne)
	if len(derived) > 32 {
		derived = derived[:32]
	}
	derivedHex := hex.EncodeToString(derived)
	sig := hmacSHA256Hex([]byte(derivedHex), []byte(stringToSign))
	original["Authorization"] = fmt.Sprintf("V11-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		ak, scope, strings.Join(signedHeaders, ";"), sig)
	return original, nil
}

func collapseSpaces(input string) string {
	return strings.Join(strings.Fields(input), " ")
}
