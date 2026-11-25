package qoder

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

const qoderRSAPublicKey = `-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDA8iMH5c02LilrsERw9t6Pv5Nc
4k6Pz1EaDicBMpdpxKduSZu5OANqUq8er4GM95omAGIOPOh+Nx0spthYA2BqGz+l
6HRkPJ7S236FZz73In/KVuLnwI8JJ2CbuJap8kvheCCZpmAWpb/cPx/3Vr/J6I17
XcW+ML9FoCI6AOvOzwIDAQAB
-----END PUBLIC KEY-----`

const qwenworkRSAPublicKey = qoderRSAPublicKey

const bodyCodecAlphabet = "_doRTgHZBKcGVjlvpC,@aFSx#DPuNJme&i*MzLOEn)sUrthbf%Y^w.(kIQyXqWA!"

var bodyCodecIndex [256]int

func init() {
	for idx := range bodyCodecIndex {
		bodyCodecIndex[idx] = -1
	}
	for idx := 0; idx < len(bodyCodecAlphabet); idx++ {
		bodyCodecIndex[bodyCodecAlphabet[idx]] = idx
	}
}

func machineOS(arch, platform string) string {
	mappedArch := arch
	switch arch {
	case "x64", "amd64":
		mappedArch = "x86_64"
	case "arm64":
		mappedArch = "aarch64"
	}
	mappedPlatform := platform
	if mappedPlatform == "windows" {
		mappedPlatform = "win32"
	}
	return mappedArch + "_" + mappedPlatform
}

func isHostSafe(value string) bool {
	if len(value) == 0 {
		return false
	}
	for idx := 0; idx < len(value); idx++ {
		ch := value[idx]
		if idx == 0 || idx == len(value)-1 {
			if ch < 0x21 || ch > 0x7e {
				return false
			}
		} else {
			if ch < 0x20 || ch > 0x7e {
				return false
			}
		}
	}
	return true
}

func hostHash8(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:8]
}

func capHostname(value string) string {
	if len(value) <= 96 {
		return value
	}
	head := strings.TrimRight(value[:96-8-1], "- \t\r\n")
	if head != "" {
		return head + "-" + hostHash8(value)
	}
	return "unknown-" + hostHash8(value)
}

func punycodeEncodeLabel(label string) (string, bool) {
	const (
		base        = 36
		tmin        = 1
		tmax        = 26
		skew        = 38
		damp        = 700
		initialBias = 72
		initialN    = 128
	)
	runes := []rune(label)
	hasNonASCII := false
	for _, char := range runes {
		if char >= 0x80 {
			hasNonASCII = true
			break
		}
	}
	if !hasNonASCII {
		return label, true
	}
	encodeDigit := func(digit int) byte {
		if digit < 26 {
			return byte('a' + digit)
		}
		return byte('0' + (digit - 26))
	}
	adapt := func(delta, numPoints int, firstTime bool) int {
		if firstTime {
			delta /= damp
		} else {
			delta /= 2
		}
		delta += delta / numPoints
		step := 0
		for delta > ((base-tmin)*tmax)/2 {
			delta /= base - tmin
			step += base
		}
		return step + ((base-tmin+1)*delta)/(delta+skew)
	}

	var out bytes.Buffer
	out.WriteString("xn--")
	basicCount := 0
	for _, char := range runes {
		if char < 0x80 {
			out.WriteByte(byte(unicode.ToLower(char)))
			basicCount++
		}
	}
	handled := basicCount
	if basicCount > 0 {
		out.WriteByte('-')
	}
	curN := initialN
	delta := 0
	bias := initialBias
	for handled < len(runes) {
		minRune := rune(0x7fffffff)
		for _, char := range runes {
			if int(char) >= curN && char < minRune {
				minRune = char
			}
		}
		delta += (int(minRune) - curN) * (handled + 1)
		curN = int(minRune)
		for _, char := range runes {
			if int(char) < curN {
				delta++
			}
			if int(char) == curN {
				quot := delta
				for step := base; ; step += base {
					thresh := step - bias
					if thresh < tmin {
						thresh = tmin
					} else if thresh > tmax {
						thresh = tmax
					}
					if quot < thresh {
						break
					}
					out.WriteByte(encodeDigit(thresh + (quot-thresh)%(base-thresh)))
					quot = (quot - thresh) / (base - thresh)
				}
				out.WriteByte(encodeDigit(quot))
				bias = adapt(delta, handled+1, handled == basicCount)
				delta = 0
				handled++
			}
		}
		delta++
		curN++
	}
	return out.String(), true
}

func domainToASCII(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	labels := strings.Split(trimmed, ".")
	encoded := make([]string, len(labels))
	for idx, label := range labels {
		if label == "" {
			encoded[idx] = ""
			continue
		}
		asciiLabel, ok := punycodeEncodeLabel(strings.ToLower(label))
		if !ok {
			return ""
		}
		encoded[idx] = asciiLabel
	}
	return strings.Join(encoded, ".")
}

func machineHostname(rawName string) string {
	trimmed := strings.TrimSpace(rawName)
	if trimmed == "" {
		return ""
	}
	if isHostSafe(trimmed) {
		return capHostname(trimmed)
	}
	ascii := domainToASCII(trimmed)
	if ascii != "" && isHostSafe(ascii) {
		return capHostname(ascii)
	}
	var safeBuilder strings.Builder
	lastDash := false
	for idx := 0; idx < len(trimmed); idx++ {
		ch := trimmed[idx]
		if ch >= 0x21 && ch <= 0x7e {
			safeBuilder.WriteByte(ch)
			lastDash = false
		} else if !lastDash {
			safeBuilder.WriteByte('-')
			lastDash = true
		}
	}
	safe := strings.Trim(safeBuilder.String(), "-")
	if safe != "" {
		return capHostname(safe + "-" + hostHash8(trimmed))
	}
	return capHostname("unknown-" + hostHash8(trimmed))
}

func parseRSAPublicKey(pemText string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemText))
	if block == nil {
		return nil, fmt.Errorf("qoder: invalid RSA public key PEM")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("qoder: public key is not RSA")
	}
	return rsaPub, nil
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padLen := blockSize - (len(data) % blockSize)
	out := make([]byte, len(data)+padLen)
	copy(out, data)
	for idx := len(data); idx < len(out); idx++ {
		out[idx] = byte(padLen)
	}
	return out
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, fmt.Errorf("invalid padded length")
	}
	padLen := int(data[len(data)-1])
	if padLen <= 0 || padLen > blockSize || padLen > len(data) {
		return nil, fmt.Errorf("invalid padding byte")
	}
	for idx := len(data) - padLen; idx < len(data); idx++ {
		if int(data[idx]) != padLen {
			return nil, fmt.Errorf("corrupt padding")
		}
	}
	return data[:len(data)-padLen], nil
}

type wrappedUserBlob struct {
	Info string
	Key  string
}

func buildUserBlob(cred Credential, rsaKeyPEM string) (wrappedUserBlob, error) {
	if rsaKeyPEM == "" {
		rsaKeyPEM = qoderRSAPublicKey
	}
	rsaPub, err := parseRSAPublicKey(rsaKeyPEM)
	if err != nil {
		return wrappedUserBlob{}, err
	}
	payloadBytes, err := json.Marshal(struct {
		UID                string `json:"uid"`
		AID                string `json:"aid"`
		Name               string `json:"name"`
		Email              string `json:"email"`
		SecurityOAuthToken string `json:"security_oauth_token"`
	}{
		UID:                cred.UID,
		AID:                "",
		Name:               cred.Name,
		Email:              cred.Email,
		SecurityOAuthToken: cred.Access,
	})
	if err != nil {
		return wrappedUserBlob{}, err
	}
	aesKey := []byte(hexID()[:16])
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return wrappedUserBlob{}, err
	}
	padded := pkcs7Pad(payloadBytes, aes.BlockSize)
	encrypted := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, aesKey).CryptBlocks(encrypted, padded)
	wrappedKey, err := rsa.EncryptPKCS1v15(rand.Reader, rsaPub, aesKey)
	if err != nil {
		return wrappedUserBlob{}, err
	}
	return wrappedUserBlob{
		Info: base64.StdEncoding.EncodeToString(encrypted),
		Key:  base64.StdEncoding.EncodeToString(wrappedKey),
	}, nil
}

func (owner *module) cosyHeaders(rawURL string, cred Credential, body string, site Site, tsSec int64) (http.Header, error) {
	if strings.TrimSpace(cred.MachineID) == "" {
		siteName := site.Name
		if siteName == "" {
			siteName = "Qoder"
		}
		return nil, fmt.Errorf("%s: the account has no machine id; sign in again", siteName)
	}
	cosyVer := site.CosyVersion
	if cosyVer == "" {
		cosyVer = cosyVersionDefault
	}
	blob, err := buildUserBlob(cred, site.RSAKey)
	if err != nil {
		return nil, err
	}
	payloadJSON, err := json.Marshal(struct {
		Version     string `json:"version"`
		RequestID   string `json:"requestId"`
		Info        string `json:"info"`
		CosyVersion string `json:"cosyVersion"`
		IDEVersion  string `json:"ideVersion"`
	}{
		Version:     "v1",
		RequestID:   randomUUID(),
		Info:        blob.Info,
		CosyVersion: cosyVer,
		IDEVersion:  "",
	})
	if err != nil {
		return nil, err
	}
	payloadB64 := base64.StdEncoding.EncodeToString(payloadJSON)
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	signPath := parsedURL.Path
	if strings.HasPrefix(signPath, "/algo") {
		signPath = signPath[5:]
	}
	sigInput := payloadB64 + "\n" + blob.Key + "\n" + strconv.FormatInt(tsSec, 10) + "\n" + body + "\n" + signPath
	sum := md5.Sum([]byte(sigInput))
	sig := hex.EncodeToString(sum[:])

	businessProduct := site.BusinessProduct
	if businessProduct == "" {
		businessProduct = "app"
	}
	clientType := site.ClientType
	if clientType == "" {
		clientType = "10"
	}
	scene := site.ModelScene
	if scene == "" {
		scene = "app"
	}

	hdr := http.Header{}
	hdr.Set("Accept", "application/json")
	hdr.Set("Accept-Encoding", "identity")
	hdr.Set("Content-Type", "application/json")
	hdr.Set("Authorization", "Bearer COSY."+payloadB64+"."+sig)
	hdr.Set("Cosy-Business-Product", businessProduct)
	hdr.Set("Cosy-Business-Type", "agent")
	hdr.Set("Cosy-ClientIp", cred.MachineID)
	hdr.Set("Cosy-ClientType", clientType)
	hdr.Set("Cosy-Data-Policy", "disagree")
	hdr.Set("Cosy-Date", strconv.FormatInt(tsSec, 10))
	hdr.Set("Cosy-Key", blob.Key)
	hdr.Set("Cosy-MachineId", cred.MachineID)
	hdr.Set("Cosy-MachineToken", cred.MachineID)
	hdr.Set("Cosy-MachineType", "5")
	hdr.Set("Cosy-MachineOS", machineOS(owner.options.Arch, owner.options.Platform))
	hdr.Set("Cosy-Scene", scene)
	hdr.Set("Cosy-User", cred.UID)
	hdr.Set("Cosy-Version", cosyVer)
	hdr.Set("Login-Version", "v2")
	return hdr, nil
}

func (owner *module) chatHeaders(rawURL string, cred Credential, body string, site Site, model ModelEntry, identity map[string]string) (http.Header, error) {
	hdr, err := owner.cosyHeaders(rawURL, cred, body, site, owner.now().Unix())
	if err != nil {
		return nil, err
	}
	hdr.Del("Cosy-ClientIp")
	hdr.Del("Cosy-MachineToken")
	hdr.Del("Cosy-MachineType")
	for _, key := range []string{"Cosy-MachineToken", "Cosy-MachineType"} {
		if val := strings.TrimSpace(identity[key]); val != "" {
			hdr.Set(key, val)
		}
	}
	if host := machineHostname(owner.options.Hostname()); host != "" {
		hdr.Set("Cosy-MachineHostname", host)
	}
	hdr.Set("Accept", "text/event-stream")
	hdr.Set("Cache-Control", "no-cache")
	hdr.Set("X-Model-Key", model.Key)
	hdr.Set("X-Model-Source", model.Source)
	return hdr, nil
}

func (owner *module) lingmaHeaders(rawURL string, cred Credential, body string, site Site) (http.Header, error) {
	hdr, err := owner.cosyHeaders(rawURL, cred, body, site, owner.now().Unix())
	if err != nil {
		return nil, err
	}
	hdr.Del("Cosy-Scene")
	hdr.Del("Cosy-Business-Product")
	hdr.Del("Cosy-Business-Type")
	hdr.Set("Accept", "text/event-stream")
	hdr.Set("Cache-Control", "no-cache")
	hdr.Set("x-request-id", randomUUID())
	return hdr, nil
}

func segmentEncode(raw []byte) string {
	var out strings.Builder
	out.Grow(((len(raw) + 2) / 3) * 4)
	acc := 0
	bits := 0
	for _, octet := range raw {
		acc = ((acc << 8) | int(octet)) & 0xffffff
		bits += 8
		for bits >= 6 {
			bits -= 6
			out.WriteByte(bodyCodecAlphabet[(acc>>bits)&63])
		}
	}
	if bits > 0 {
		out.WriteByte(bodyCodecAlphabet[(acc<<(6-bits))&63])
	}
	for out.Len()%4 != 0 {
		out.WriteByte('$')
	}
	return out.String()
}

func swapThirds(encoded string) string {
	third := len(encoded) / 3
	if third == 0 {
		return encoded
	}
	return encoded[len(encoded)-third:] + encoded[third:len(encoded)-third] + encoded[:third]
}

func encodeBody(plain string) string {
	return swapThirds(segmentEncode([]byte(plain)))
}

func decodeBody(wire string) string {
	unswapped := swapThirds(wire)
	out := make([]byte, 0, (len(unswapped)/4)*3)
	for group := 0; group+4 <= len(unswapped); group += 4 {
		var vals [4]int
		count := 0
		for pos := 0; pos < 4; pos++ {
			ch := unswapped[group+pos]
			if ch == '$' {
				continue
			}
			idx := bodyCodecIndex[ch]
			if idx < 0 {
				idx = 0
			}
			vals[count] = idx
			count++
		}
		acc := uint64(0)
		for idx := 0; idx < count; idx++ {
			acc = acc*64 + uint64(vals[idx])
		}
		byteCount := (6 * count) / 8
		for idx := 0; idx < byteCount; idx++ {
			shift := uint(6*count - 8*(idx+1))
			out = append(out, byte((acc>>shift)&0xff))
		}
	}
	return string(out)
}
