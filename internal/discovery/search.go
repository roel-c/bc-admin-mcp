package discovery

import (
	"fmt"
	"sort"
	"strings"
)

const (
	maxSearchResults = 25
	maxSuggestions   = 5
)

// Search finds categories and tools whose path or summary contains query
// (case-insensitive). When scope is non-empty it must be a registered category;
// only paths equal to scope or under scope/ are returned.
func (r *Registry) Search(query, scope string) ([]DiscoveryEntry, error) {
	q := strings.TrimSpace(strings.ToLower(query))
	if q == "" {
		return nil, fmt.Errorf("query must not be empty")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if scope != "" {
		if _, ok := r.categories[scope]; !ok {
			return nil, fmt.Errorf(
				"scope path %q is not a category; omit path for a global search or pass a valid category like \"catalog\"",
				scope,
			)
		}
	}

	type scored struct {
		entry DiscoveryEntry
		score int
	}
	var hits []scored

	for _, cat := range r.categories {
		if !inScope(cat.Path, scope) {
			continue
		}
		if s, ok := matchScore(q, cat.Path, cat.Summary); ok {
			hits = append(hits, scored{
				entry: DiscoveryEntry{Path: cat.Path, Type: "category", Summary: cat.Summary},
				score: s,
			})
		}
	}
	for _, tool := range r.tools {
		if !inScope(tool.Path, scope) {
			continue
		}
		if s, ok := matchScore(q, tool.Path, tool.Summary); ok {
			hits = append(hits, scored{
				entry: DiscoveryEntry{
					Path:    tool.Path,
					Type:    "tool",
					Summary: tool.Summary,
					Tier:    string(tool.Tier),
				},
				score: s,
			})
		}
	}

	sort.Slice(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].entry.Path < hits[j].entry.Path
	})

	if len(hits) > maxSearchResults {
		hits = hits[:maxSearchResults]
	}
	out := make([]DiscoveryEntry, len(hits))
	for i := range hits {
		out[i] = hits[i].entry
	}
	return out, nil
}

// suggestPaths returns up to limit path strings that best match needle
// (used for did-you-mean on Discover / execute_tool misses).
// Caller must hold at least r.mu.RLock.
func (r *Registry) suggestPaths(needle string, limit int) []string {
	q := strings.TrimSpace(strings.ToLower(needle))
	if q == "" || limit <= 0 {
		return nil
	}

	type scored struct {
		path  string
		score int
	}
	var hits []scored

	for _, cat := range r.categories {
		if s, ok := matchScore(q, cat.Path, cat.Summary); ok {
			hits = append(hits, scored{path: cat.Path, score: s})
		}
	}
	for _, tool := range r.tools {
		if s, ok := matchScore(q, tool.Path, tool.Summary); ok {
			hits = append(hits, scored{path: tool.Path, score: s})
		}
	}

	sort.Slice(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].path < hits[j].path
	})

	seen := make(map[string]struct{}, limit)
	out := make([]string, 0, limit)
	for _, h := range hits {
		if _, ok := seen[h.path]; ok {
			continue
		}
		seen[h.path] = struct{}{}
		out = append(out, h.path)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func inScope(path, scope string) bool {
	if scope == "" {
		return true
	}
	return path == scope || strings.HasPrefix(path, scope+"/")
}

// matchScore ranks how well path/summary match q. Higher is better.
// Returns ok=false when there is no useful match.
func matchScore(q, path, summary string) (score int, ok bool) {
	lp := strings.ToLower(path)
	ls := strings.ToLower(summary)

	if lp == q {
		return 100, true
	}
	// Final path segment exact match (e.g. query "channels" → catalog/channels).
	if seg := pathSegment(lp); seg == q {
		return 90, true
	}
	if strings.HasSuffix(lp, "/"+q) {
		return 85, true
	}
	if strings.Contains(lp, "/"+q+"/") {
		return 75, true
	}
	if strings.Contains(lp, q) {
		return 60, true
	}
	if ls != "" && strings.Contains(ls, q) {
		return 40, true
	}
	// Multi-token: all tokens appear somewhere in path or summary.
	tokens := strings.FieldsFunc(q, func(r rune) bool {
		return r == ' ' || r == '/' || r == '_' || r == '-'
	})
	if len(tokens) > 1 {
		hay := lp + " " + ls
		for _, t := range tokens {
			if t == "" || !strings.Contains(hay, t) {
				return 0, false
			}
		}
		return 30, true
	}
	return 0, false
}

func pathSegment(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

func formatNotFound(kind, path string, suggestions []string) string {
	if len(suggestions) == 0 {
		return fmt.Sprintf(
			`%s %q not found; pass query=%q to search the tool hierarchy`,
			kind, path, path,
		)
	}
	return fmt.Sprintf(
		`%s %q not found; did you mean: %s (or pass query=%q to search)`,
		kind, path, strings.Join(suggestions, ", "), path,
	)
}
