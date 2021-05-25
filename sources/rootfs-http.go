package sources

import (
        "fmt"
        "net/http"
        "net/url"
        "path"
        "path/filepath"

        lxd "github.com/lxc/lxd/shared"
        "github.com/pkg/errors"

        "github.com/lxc/distrobuilder/shared"
)

// RootFsHTTP represents the Sabayon Linux downloader.
type RootFsHTTP struct{}

// NewRootFsHTTP creates a new RootFsHTTP instance.
func NewRootFsHTTP() *RootFsHTTP {
        return &RootFsHTTP{}
}

// Run downloads a Sabayon tarball.
func (s *RootFsHTTP) Run(definition shared.Definition, rootfsDir string) error {
        tarballPath := definition.Source.URL

        resp, err := http.Head(tarballPath)
        if err != nil {
                return errors.Wrap(err, "Couldn't resolve URL")
        }

        baseURL, fname := path.Split(resp.Request.URL.String())

        url, err := url.Parse(fmt.Sprintf("%s/%s", baseURL, fname))
        if err != nil {
                return err
        }

        var fpath string
        fpath, err = shared.DownloadHash(definition.Image, url.String(), "", nil)
        if err != nil {
                return err
        }

        // Unpack
        err = lxd.Unpack(filepath.Join(fpath, fname), rootfsDir, false, false, nil)
        if err != nil {
                return err
        }

        return nil
}
