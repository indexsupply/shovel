package main

import (
	"bytes"
	"crypto/rand"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudfront"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	ghtml "github.com/yuin/goldmark/renderer/html"
	"golang.org/x/net/html"
)

var (
	deploy bool
	dir    string
	bucket string
	distid string
	sess   *session.Session
)

func check(err error) {
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func main() {
	flag.StringVar(&bucket, "bucket", "indexsupply.com", "s3 bucket")
	flag.BoolVar(&deploy, "deploy", false, "deploy to s3")
	flag.StringVar(&dir, "dir", "indexsupply.com", "root dir")
	flag.StringVar(&distid, "distid", "E100U1X0OYQONF", "cloudfront distribution")
	flag.Parse()

	sess = session.Must(session.NewSession())

	if deploy {
		check(filepath.Walk(dir, upload))
		check(invalidate())
		return
	}
	http.HandleFunc("/", serve)
	http.ListenAndServe(":8080", nil)
}

func isdir(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.IsDir()
}

func serve(w http.ResponseWriter, r *http.Request) {
	p := path.Join(dir, r.URL.Path)
	if isdir(p) {
		p = path.Join(p, "index.md")
	} else {
		p += ".md"
	}
	f, err := os.Open(p)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	res, err := render(f)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	fmt.Fprint(w, string(res))
}

func render(f *os.File) ([]byte, error) {
	src, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", f.Name(), err)
	}

	var buf bytes.Buffer
	md := goldmark.New(
		goldmark.WithRendererOptions(
			ghtml.WithHardWraps(),
			ghtml.WithUnsafe(),
			ghtml.WithXHTML(),
		),
		goldmark.WithExtensions(extension.Typographer),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
	)
	err = md.Convert(src, &buf)
	if err != nil {
		return nil, fmt.Errorf("md converting %s: %w", f.Name(), err)
	}
	res, err := layout(f.Name(), buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("adding layout %s: %w", f.Name(), err)
	}
	return title(res), nil
}

func title(body []byte) []byte {
	doc, err := html.Parse(bytes.NewReader(body))
	check(err)

	var title string
	var f func(*html.Node) *html.Node
	f = func(n *html.Node) *html.Node {
		if n.Type == html.ElementNode && n.Data == "title" {
			title = n.FirstChild.Data
			return n
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if d := f(c); d != nil {
				n.RemoveChild(d)
			}
		}
		return nil
	}
	f(doc)
	return bytes.Replace(body, []byte("{{Title}}"), []byte(title), 1)
}

func layout(p string, src []byte) ([]byte, error) {
	var (
		dirs = strings.Split(path.Dir(p), "/")
		l    []byte
	)
	for i := len(dirs); i >= 0 && len(l) == 0; i-- {
		dir := path.Join(dirs[:i]...)
		l, _ = os.ReadFile(path.Join(dir, "_layout.html"))
		if len(l) > 0 {
			break
		}
	}
	if len(l) == 0 {
		return nil, fmt.Errorf("unable to find a _layout.html file")
	}
	return bytes.Replace(l, []byte("{{Body}}"), src, 1), nil
}

func upload(p string, fi os.FileInfo, err error) error {
	switch {
	case err != nil:
		return err
	case strings.HasPrefix(fi.Name(), "_"):
		return nil
	case fi.Name() == "index.md":
		return nil
	case fi.IsDir():
		f, err := os.Open(path.Join(p, "index.md"))
		if err != nil {
			fmt.Printf("skipping %s: %s\n", f.Name(), err)
			return nil
		}
		defer f.Close()
		b, err := render(f)
		if err != nil {
			return fmt.Errorf("rendering file %s: %w", f.Name(), err)
		}
		return putFile(b, bucket, key(p))
	case filepath.Ext(p) == ".md":
		f, err := os.Open(p)
		if err != nil {
			fmt.Printf("skipping %s: %s\n", f.Name(), err)
			return nil
		}
		defer f.Close()
		b, err := render(f)
		if err != nil {
			return fmt.Errorf("rendering file %s: %w", f.Name(), err)
		}
		return putFile(b, bucket, key(p))
	default:
		return nil
	}
}

func key(fileName string) string {
	k := strings.TrimSuffix(
		strings.TrimPrefix(
			fileName,
			dir,
		),
		filepath.Ext(fileName),
	)
	if k == "" {
		k = "index.html"
	}
	return k
}

func putFile(b []byte, bucket, key string) error {
	_, err := s3manager.NewUploader(sess).Upload(&s3manager.UploadInput{
		Bucket:      &bucket,
		Key:         &key,
		Body:        bytes.NewReader(b),
		ContentType: aws.String(http.DetectContentType(b)),
	})
	return err
}

func invalidate() error {
	var ref [1 << 5]byte
	rand.Read(ref[:])
	_, err := cloudfront.New(sess).CreateInvalidation(&cloudfront.CreateInvalidationInput{
		DistributionId: &distid,
		InvalidationBatch: &cloudfront.InvalidationBatch{
			CallerReference: aws.String(string(ref[:])),
			Paths: &cloudfront.Paths{
				Quantity: aws.Int64(1),
				Items:    []*string{aws.String("/*")},
			},
		},
	})
	return err
}
