package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type State struct {
	CurrentVersion string    `json:"current_version"`
	LatestVersion  string    `json:"latest_version,omitempty"`
	Channel        string    `json:"channel"`
	Available      bool      `json:"available"`
	PackageURL     string    `json:"package_url,omitempty"`
	SHA256         string    `json:"sha256,omitempty"`
	Notes          string    `json:"notes,omitempty"`
	CheckedAt      time.Time `json:"checked_at,omitempty"`
	Error          string    `json:"error,omitempty"`
}

type Checker struct {
	mu          sync.RWMutex
	state       State
	manifestURL string
	onAvailable func(State)
	client      *http.Client
}

var shaPattern = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)

func NewChecker(version, channel, manifestURL string, onAvailable func(State)) *Checker {
	return &Checker{
		state:       State{CurrentVersion: version, Channel: channel},
		manifestURL: manifestURL,
		onAvailable: onAvailable,
		client:      &http.Client{Timeout: 15 * time.Second},
	}
}

func (checker *Checker) State() State {
	checker.mu.RLock()
	defer checker.mu.RUnlock()
	return checker.state
}

func (checker *Checker) Check(ctx context.Context) error {
	if checker.manifestURL == "" {
		return nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, checker.manifestURL, nil)
	if err != nil {
		return checker.fail(err)
	}
	response, err := checker.client.Do(request)
	if err != nil {
		return checker.fail(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return checker.fail(fmt.Errorf("manifest status %d", response.StatusCode))
	}
	var manifest struct {
		Version    string `json:"version"`
		Channel    string `json:"channel"`
		PackageURL string `json:"package_url"`
		SHA256     string `json:"sha256"`
		Notes      string `json:"notes"`
	}
	if err := json.NewDecoder(response.Body).Decode(&manifest); err != nil {
		return checker.fail(err)
	}
	checker.mu.Lock()
	current := checker.state
	if manifest.Channel != current.Channel || !validVersion(manifest.Version) || manifest.PackageURL == "" || !shaPattern.MatchString(manifest.SHA256) {
		checker.mu.Unlock()
		return checker.fail(fmt.Errorf("invalid update manifest"))
	}
	available := newer(manifest.Version, current.CurrentVersion)
	changed := available && (!current.Available || current.LatestVersion != manifest.Version)
	checker.state.LatestVersion = manifest.Version
	checker.state.Available = available
	checker.state.PackageURL = manifest.PackageURL
	checker.state.SHA256 = manifest.SHA256
	checker.state.Notes = manifest.Notes
	checker.state.CheckedAt = time.Now().UTC()
	checker.state.Error = ""
	state := checker.state
	checker.mu.Unlock()
	if changed && checker.onAvailable != nil {
		checker.onAvailable(state)
	}
	return nil
}

func (checker *Checker) fail(err error) error {
	checker.mu.Lock()
	checker.state.Error = err.Error()
	checker.state.CheckedAt = time.Now().UTC()
	checker.mu.Unlock()
	return err
}

func validVersion(version string) bool {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if _, err := strconv.Atoi(part); err != nil {
			return false
		}
	}
	return true
}

func newer(latest, current string) bool {
	if !validVersion(latest) || !validVersion(current) {
		return false
	}
	left, right := strings.Split(latest, "."), strings.Split(current, ".")
	for index := range left {
		leftPart, _ := strconv.Atoi(left[index])
		rightPart, _ := strconv.Atoi(right[index])
		if leftPart != rightPart {
			return leftPart > rightPart
		}
	}
	return false
}
