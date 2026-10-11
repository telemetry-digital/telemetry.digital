package offsite

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

// s3Target speaks the S3 REST API with AWS Signature Version 4 (no SDK): PUT, GET, DELETE and ListObjectsV2. A
// single PUT holds up to 5 GiB, which is far above a bundle of the database (the recordings are not backed up).
type s3Target struct {
	c      Config
	base   *url.URL
	client *http.Client
	now    func() time.Time
}

const maxS3Put = 5 << 30

func newS3(c Config) (*s3Target, error) {
	u, err := url.Parse(strings.TrimRight(c.Endpoint, "/"))
	if err != nil {
		return nil, err
	}
	if c.Region == "" {
		c.Region = "us-east-1"
	}
	return &s3Target{c: c, base: u, client: &http.Client{Timeout: 0}, now: time.Now}, nil
}

// objectURL returns the URL of a key (path-style or virtual-hosted).
func (s *s3Target) objectURL(key string, q url.Values) *url.URL {
	u := *s.base
	if s.c.PathStyle {
		u.Path = "/" + s.c.Bucket + "/" + key
	} else {
		u.Host = s.c.Bucket + "." + u.Host
		u.Path = "/" + key
	}
	if q != nil {
		u.RawQuery = encodeQuery(q)
	}
	return &u
}

func (s *s3Target) key(name string) string {
	p := strings.Trim(s.c.Path, "/")
	if p == "" {
		return name
	}
	return p + "/" + name
}

func (s *s3Target) do(ctx context.Context, method string, u *url.URL, body io.Reader, size int64, payloadHash string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.ContentLength = size
	}
	sign(req, s.c.AccessKey, s.c.SecretKey, s.c.Region, payloadHash, s.now().UTC())
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("s3: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		defer resp.Body.Close()
		return nil, s3Error(resp)
	}
	return resp, nil
}

func s3Error(resp *http.Response) error {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var e struct {
		Code    string `xml:"Code"`
		Message string `xml:"Message"`
	}
	if xml.Unmarshal(b, &e) == nil && e.Code != "" {
		return fmt.Errorf("s3: %s %s: %s", resp.Status, e.Code, e.Message)
	}
	return fmt.Errorf("s3: %s", resp.Status)
}

func (s *s3Target) Put(ctx context.Context, name, localPath string) error {
	if err := checkName(name); err != nil {
		return err
	}
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	if st.Size() > maxS3Put {
		return errors.New("s3: the file is larger than 5 GiB (single upload limit)")
	}
	h := sha256.New()
	if _, err := io.Copy(h, ctxReader{ctx, f}); err != nil {
		return err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	resp, err := s.do(ctx, http.MethodPut, s.objectURL(s.key(name), nil), ctxReader{ctx, f}, st.Size(), hex.EncodeToString(h.Sum(nil)))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (s *s3Target) Get(ctx context.Context, name string) (io.ReadCloser, error) {
	if err := checkName(name); err != nil {
		return nil, err
	}
	resp, err := s.do(ctx, http.MethodGet, s.objectURL(s.key(name), nil), nil, 0, emptySHA256)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (s *s3Target) List(ctx context.Context) ([]Object, error) {
	prefix := strings.Trim(s.c.Path, "/")
	if prefix != "" {
		prefix += "/"
	}
	var out []Object
	token := ""
	for page := 0; page < 100; page++ {
		q := url.Values{"list-type": {"2"}, "prefix": {prefix + "ctrl32-"}}
		if token != "" {
			q.Set("continuation-token", token)
		}
		u := s.objectURL("", q)
		resp, err := s.do(ctx, http.MethodGet, u, nil, 0, emptySHA256)
		if err != nil {
			return nil, err
		}
		var r struct {
			Contents []struct {
				Key          string    `xml:"Key"`
				Size         int64     `xml:"Size"`
				LastModified time.Time `xml:"LastModified"`
			} `xml:"Contents"`
			IsTruncated           bool   `xml:"IsTruncated"`
			NextContinuationToken string `xml:"NextContinuationToken"`
		}
		err = xml.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&r)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("s3: list: %w", err)
		}
		for _, c := range r.Contents {
			n := strings.TrimPrefix(c.Key, prefix)
			if ValidName(n) {
				out = append(out, Object{Name: n, Size: c.Size, Modified: c.LastModified})
			}
		}
		if !r.IsTruncated || r.NextContinuationToken == "" {
			return out, nil
		}
		token = r.NextContinuationToken
	}
	return out, nil
}

func (s *s3Target) Delete(ctx context.Context, name string) error {
	if err := checkName(name); err != nil {
		return err
	}
	resp, err := s.do(ctx, http.MethodDelete, s.objectURL(s.key(name), nil), nil, 0, emptySHA256)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (s *s3Target) Close() error { return nil }

// ---- AWS Signature Version 4 ------------------------------------------------------------------------------------

const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// sign adds x-amz-date, x-amz-content-sha256 and Authorization. Signed headers: host, range (when present) and the
// x-amz-* headers.
func sign(req *http.Request, accessKey, secretKey, region, payloadHash string, now time.Time) {
	amzDate := now.Format("20060102T150405Z")
	day := now.Format("20060102")
	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", payloadHash)

	headers := map[string]string{"host": req.URL.Host}
	for k, v := range req.Header {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "x-amz-") || lk == "range" || lk == "content-md5" {
			headers[lk] = strings.TrimSpace(strings.Join(v, ","))
		}
	}
	names := make([]string, 0, len(headers))
	for k := range headers {
		names = append(names, k)
	}
	sort.Strings(names)
	var ch strings.Builder
	for _, k := range names {
		ch.WriteString(k + ":" + headers[k] + "\n")
	}
	signed := strings.Join(names, ";")
	canonical := req.Method + "\n" + encodePath(req.URL.Path) + "\n" + canonicalQuery(req.URL.Query()) + "\n" + ch.String() + "\n" + signed + "\n" + payloadHash
	scope := day + "/" + region + "/s3/aws4_request"
	sum := sha256.Sum256([]byte(canonical))
	toSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(sum[:])
	k := hmacSHA256([]byte("AWS4"+secretKey), day)
	k = hmacSHA256(k, region)
	k = hmacSHA256(k, "s3")
	k = hmacSHA256(k, "aws4_request")
	sig := hex.EncodeToString(hmacSHA256(k, toSign))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+accessKey+"/"+scope+", SignedHeaders="+signed+", Signature="+sig)
}

func hmacSHA256(key []byte, data string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(data))
	return m.Sum(nil)
}

// uriEncode is the strict encoding of SigV4: everything but A–Z a–z 0–9 - _ . ~ is percent-encoded (and "/" too
// unless keepSlash).
func uriEncode(s string, keepSlash bool) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.' || c == '~' || c == '/' && keepSlash {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func encodePath(p string) string {
	if p == "" {
		return "/"
	}
	return uriEncode(p, true)
}

func canonicalQuery(q url.Values) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		vs := append([]string(nil), q[k]...)
		sort.Strings(vs)
		for _, v := range vs {
			parts = append(parts, uriEncode(k, false)+"="+uriEncode(v, false))
		}
	}
	return strings.Join(parts, "&")
}

// encodeQuery builds the request's query with the same encoding the signature uses.
func encodeQuery(q url.Values) string { return canonicalQuery(q) }
