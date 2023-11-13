package main

import (
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudfront"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

func check(err error) {
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func main() {
	var (
		distID string
		bucket string
		tmpdir string
		cmd    string
		tag    string
		awss   = session.Must(session.NewSession())
	)
	flag.StringVar(&bucket, "bucket", "bin.indexsupply.net", "s3 bucket")
	flag.StringVar(&cmd, "cmd", "", "command to build")
	flag.StringVar(&tag, "tag", "main", "version tag")
	flag.StringVar(&tmpdir, "dir", "/tmp", "tmp binary storage")
	flag.StringVar(&distID, "dist", "", "aws cloudfront dist to invalidate")
	flag.Parse()

	fmt.Printf("tag: %s\n", tag)
	os.Exit(1)

	if cmd == "" {
		check(errors.New("mimssing cmd flag"))
	}

	check(os.MkdirAll(tmpdir, 0750))

	var platforms = [][]string{
		[]string{"linux", "amd64"},
		[]string{"darwin", "arm64"},
		[]string{"darwin", "amd64"},
		[]string{"windows", "amd64"},
	}

	fmt.Println("released:")
	for i := range platforms {
		var (
			goos    = platforms[i][0]
			goarch  = platforms[i][1]
			binPath = fmt.Sprintf("%s/%s/%s/%s", tmpdir, goos, goarch, cmd)
			cmdPath = fmt.Sprintf("./cmd/%s", cmd)
			s3key   = fmt.Sprintf("bin/%s/%s/%s/%s", tag, goos, goarch, cmd)
		)
		check(build(goos, goarch, binPath, cmdPath))
		f, err := os.Open(binPath)
		check(err)
		check(putFile(awss, f, bucket, s3key))
		f.Close()
		if distID != "" {
			check(invalidate(tag, awss, distID))
		}
		fmt.Printf("- %s/%s\n", bucket, s3key)
	}
}

func hash(f *os.File) string {
	h := sha256.New()
	_, err := io.Copy(h, f)
	check(err)
	return fmt.Sprintf("%x", h.Sum(nil))
}

func build(goos, goarch, binPath, cmdPath string) error {
	var out strings.Builder
	c := exec.Command("go", "build", "-o", binPath, cmdPath)
	c.Stdout = &out
	c.Stderr = &out
	c.Env = append(os.Environ(),
		fmt.Sprintf("GOOS=%s", goos),
		fmt.Sprintf("GOARCH=%s", goarch),
	)
	err := c.Run()
	if err != nil {
		fmt.Printf("err=%s out=%s\n", err, out.String())
		return err
	}
	return nil
}

func putFile(s *session.Session, f *os.File, bucket, key string) error {
	_, err := s3manager.NewUploader(s).Upload(&s3manager.UploadInput{
		Bucket: &bucket,
		Key:    &key,
		Body:   f,
	})
	return err
}

func randstr() string {
	var b [1 << 5]byte
	rand.Read(b[:])
	return string(b[:])
}

// since we overwrite the binaries in the 'main' dir
// we need to inform cloudfront's cache of the change
func invalidate(tag string, s *session.Session, distID string) error {
	var paths = []*string{aws.String(fmt.Sprintf("/bin/%s/*", tag))}
	_, err := cloudfront.New(s).CreateInvalidation(&cloudfront.CreateInvalidationInput{
		DistributionId: &distID,
		InvalidationBatch: &cloudfront.InvalidationBatch{
			CallerReference: aws.String(randstr()),
			Paths: &cloudfront.Paths{
				Quantity: aws.Int64(int64(len(paths))),
				Items:    paths,
			},
		},
	})
	return err
}
