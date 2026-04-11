package github

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/sync/ports"
)

type DataSourceRepo struct {
	client *http.Client
	token  string
}

func NewDataSourceRepo(token string) *DataSourceRepo {
	return &DataSourceRepo{
		client: http.DefaultClient,
		token:  token,
	}
}

func (r *DataSourceRepo) OpenRepoArchive(input ports.OpenRepoFileStreamInput) (ports.RepoFileStream, error) {
	owner, repo, err := parseGitHubRepoURL(input.RepoURL)
	if err != nil {
		return nil, err
	}

	branch := strings.TrimSpace(input.Branch)
	commitSHA := strings.TrimSpace(input.CommitSHA)
	ref := commitSHA
	if ref == "" {
		ref = branch
	}

	if ref == "" {
		return nil, fmt.Errorf("branch or commit SHA is required")
	}

	if branch != "" && commitSHA != "" {
		matches, err := r.branchContainsCommit(owner, repo, branch, commitSHA)
		if err != nil {
			return nil, err
		}
		if !matches {
			return nil, fmt.Errorf("commit %q is not contained in branch %q for repo %s/%s", commitSHA, branch, owner, repo)
		}
	}

	archiveURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/tarball", owner, repo)
	if ref != "" {
		archiveURL = archiveURL + "/" + url.PathEscape(ref)
	}

	req, err := http.NewRequest(http.MethodGet, archiveURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create github archive request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if strings.TrimSpace(r.token) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(r.token))
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download github archive: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
		return nil, fmt.Errorf("github archive request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	gzipReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		resp.Body.Close()
		return nil, fmt.Errorf("open gzip archive: %w", err)
	}

	stream := &repoFileStream{
		responseBody: resp.Body,
		gzipReader:   gzipReader,
		tarReader:    tar.NewReader(gzipReader),
	}

	return stream, nil
}

func (r *DataSourceRepo) ComputeDiff(oldSHA string, newSHA string) (bool, error) {
	if strings.TrimSpace(oldSHA) == "" || strings.TrimSpace(newSHA) == "" {
		return false, fmt.Errorf("oldSHA and newSHA are required")
	}

	return strings.TrimSpace(oldSHA) != strings.TrimSpace(newSHA), nil
}

func (r *DataSourceRepo) branchContainsCommit(owner string, repo string, branch string, commitSHA string) (bool, error) {
	compareURL := fmt.Sprintf(
		"https://api.github.com/repos/%s/%s/compare/%s...%s",
		owner,
		repo,
		url.PathEscape(commitSHA),
		url.PathEscape(branch),
	)

	req, err := http.NewRequest(http.MethodGet, compareURL, nil)
	if err != nil {
		return false, fmt.Errorf("create github compare request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if strings.TrimSpace(r.token) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(r.token))
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("load github compare metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
		return false, fmt.Errorf("github compare request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload struct {
		Status string `json:"status"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return false, fmt.Errorf("decode github compare metadata: %w", err)
	}

	status := strings.TrimSpace(payload.Status)
	return status == "identical" || status == "ahead", nil
}

type repoFileStream struct {
	responseBody io.Closer
	gzipReader   io.Closer
	tarReader    *tar.Reader
	commitSHA    string
}

func (s *repoFileStream) Next() (*ports.RepoFile, error) {
	for {
		header, err := s.tarReader.Next()
		if err != nil {
			return nil, err
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}

		path, commitSHA, err := normalizeArchivePath(header.Name)
		if err != nil {
			return nil, err
		}

		if path == "" {
			continue
		}

		if s.commitSHA == "" && commitSHA != "" {
			s.commitSHA = commitSHA
		}

		return &ports.RepoFile{
			Path:      path,
			Extension: normalizeFileExtension(path),
			Size:      header.Size,
			Reader:    io.NopCloser(io.LimitReader(s.tarReader, header.Size)),
		}, nil
	}
}

func (s *repoFileStream) Close() error {
	var errs []error

	if s.gzipReader != nil {
		if err := s.gzipReader.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if s.responseBody != nil {
		if err := s.responseBody.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) == 0 {
		return nil
	}

	return errors.Join(errs...)
}

func (s *repoFileStream) CommitSHA() string {
	return s.commitSHA
}

func parseGitHubRepoURL(raw string) (string, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", "", fmt.Errorf("parse repo url: %w", err)
	}

	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("repo url %q is missing owner/repo", raw)
	}

	owner := parts[0]
	repo := strings.TrimSuffix(parts[1], ".git")
	if owner == "" || repo == "" {
		return "", "", fmt.Errorf("repo url %q is missing owner/repo", raw)
	}

	return owner, repo, nil
}

func normalizeArchivePath(name string) (string, string, error) {
	parts := strings.Split(strings.Trim(name, "/"), "/")
	if len(parts) < 2 {
		return "", "", nil
	}

	topLevel := parts[0]
	path := strings.Join(parts[1:], "/")
	if path == "" {
		return "", "", nil
	}

	commitSHA := ""
	if idx := strings.LastIndex(topLevel, "-"); idx >= 0 && idx < len(topLevel)-1 {
		commitSHA = topLevel[idx+1:]
	}

	return path, commitSHA, nil
}

func normalizeFileExtension(filePath string) string {
	extension := strings.TrimSpace(path.Ext(filePath))
	if extension == "" {
		return ""
	}

	return strings.TrimPrefix(extension, ".")
}
