package papertool

import (
	"context"
	"fmt"
	"golang.org/x/text/number"
	"gopkg.in/vansante/go-dl-stream.v2"
	"net/url"
	"os"
	"time"
)

func Download(serverURL *url.URL, project string, version string, build string, artifact *Artifact, dstdir string, replace bool, quiet bool) error {
	if artifact == nil || artifact.Application == nil || artifact.Application.Name == nil {
		return fmt.Errorf("bad artifact")
	}

	src := fmt.Sprintf("%s/v2/projects/%s/versions/%s/builds/%s/downloads/%s", serverURL.String(), project, version, build, String(artifact.Application.Name))
	dst := fmt.Sprintf("%s/%s", dstdir, String(artifact.Application.Name))

	_, err := os.Stat(dst)
	if err == nil {
		if !replace {
			return fmt.Errorf("%s: already exists and -replace not specified", dst)
		}

		err = os.Remove(dst)
		if err != nil {
			return fmt.Errorf("remove %s: %v", dst, err)
		}
	} else {
		if !os.IsNotExist(err) {
			return fmt.Errorf("stat %s: %v", dst, err)
		}
	}

	msg := fmt.Sprintf("%s to %s", src, dst)

	sw := NewStatusWriter(msg, quiet)

	err = dlstream.DownloadStream(context.Background(), src, dst, sw)
	if err != nil {
		return err
	}

	elapsed := time.Now().Sub(sw.start)
	kbps := float64(sw.total) / 1000.0 / elapsed.Seconds()

	hash := fmt.Sprintf("%x", sw.sha256.Sum(nil))
	sw.p.Printf("%s%sDownloaded %s to %s %v bytes (%v KB/s) sha256 %s\n", EraseLine, SOL, src, dst, number.Decimal(sw.total), sw.format(kbps), hash)

	expected := String(artifact.Application.Sha256)
	if expected == "" {
		return nil
	}

	if hash != expected {
		return fmt.Errorf("%s: sha256 mismatch %s expected %s", dst, hash, expected)
	}

	return nil
}
