package upstreams

import (
	"fmt"
	"io/ioutil"
	"os"

	log "github.com/sirupsen/logrus"
	"k8s.io/helm/pkg/getter"
	"k8s.io/helm/pkg/helm/environment"
	"k8s.io/helm/pkg/repo"
)

// Helm upstream
type Helm struct {
	UpstreamBase `mapstructure:",squash"`
	// URL of the repository
	// If left blank, defaults to "stable", i.e. https://kubernetes-charts.storage.googleapis.com/
	Repo string
	// Name of the Helm chart
	Name string
	// Optional: semver constraints, e.g. < 2.0.0
	// Will have no effect if the dependency does not follow Semver
	Constraints string
	// Optional: authentication options
	Username string
	Password string
	CertFile string
	KeyFile  string
	CAFile   string
}

// Cache remote repositories locally to prevent unnecessary network round-trips
var cache map[string]*repo.IndexFile

// getIndex returns the index for the given repository, and caches it for subsequent calls
func getIndex(c repo.Entry) (*repo.IndexFile, error) {
	// Check cache first
	if cache == nil {
		// No cache: initialise it
		cache = make(map[string]*repo.IndexFile)
	} else {
		index, cacheHit := cache[c.URL]
		if cacheHit {
			log.Debugf("Using cached index for %s", c.URL)
			return index, nil
		}
	}

	// Download and write the index file to a temporary location
	tempIndexFile, err := ioutil.TempFile("", "tmp-repo-file")
	if err != nil {
		return nil, fmt.Errorf("cannot write index file for repository requested")
	}
	defer os.Remove(tempIndexFile.Name())

	r, err := repo.NewChartRepository(&c, getter.All(environment.EnvSettings{}))
	if err != nil {
		return nil, err
	}
	if err := r.DownloadIndexFile(tempIndexFile.Name()); err != nil {
		return nil, fmt.Errorf("Looks like %q is not a valid chart repository or cannot be reached: %s", c.URL, err)
	}
	index, err := repo.LoadIndexFile(tempIndexFile.Name())
	if err != nil {
		return nil, err
	}

	// Found: add to cache
	cache[c.URL] = index
	return index, nil
}

// LatestVersion returns the latest version of a Helm chart.
//
// Returns the latest chart version in the given repository.
//
// Authentication
//
// Authentication is passed through parameters on the upstream, matching the ones you'd pass to Helm directly.
func (upstream Helm) LatestVersion() (string, error) {
	log.Debugf("Using Helm upstream")

	repoURL := upstream.Repo
	if repoURL == "" || repoURL == "stable" {
		repoURL = "https://kubernetes-charts.storage.googleapis.com/"
	}

	entry := repo.Entry{
		URL:      repoURL,
		Username: upstream.Username,
		Password: upstream.Password,
		CertFile: upstream.CertFile,
		KeyFile:  upstream.KeyFile,
		CAFile:   upstream.CAFile,
	}

	// Get the index
	index, err := getIndex(entry)
	if err != nil {
		return "", err
	}

	cv, err := index.Get(upstream.Name, upstream.Constraints)
	if err != nil {
		if upstream.Constraints != "" {
			return "", fmt.Errorf("%s not found in %s repository (with constraints: %s)", upstream.Name, repoURL, upstream.Constraints)
		}
		return "", fmt.Errorf("%s not found in %s repository", upstream.Name, repoURL)
	}

	return cv.Version, nil
}
