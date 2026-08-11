package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
)

var (
	bulkCategorySlugNonAlnum = regexp.MustCompile(`[^a-z0-9]+`)
	bulkCategorySlugTrim     = regexp.MustCompile(`^-+|-+$`)
)

const (
	// maxBulkCategoryCreateNodes caps total categories (including nested children)
	// in one catalog/categories/bulk_create call.
	maxBulkCategoryCreateNodes = 100
	// maxBulkCategoryCreateDepth matches BigCommerce's documented 8-level limit.
	maxBulkCategoryCreateDepth = 8
)

// registerBulkCreateTool is called from Categories.RegisterTools.
func (c *Categories) registerBulkCreateTool(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/categories/bulk_create",
		Tier:    middleware.TierR1,
		Summary: "Create a category tree in one call (level-by-level batch POST)",
		Description: "Creates many categories under one preview→confirm. Pass categories_json: a JSON " +
			"array of nodes with name and optional nested children (preferred for mega-menus), or flat " +
			"nodes linked by ref/parent_ref. Attach to an existing store category with parent_id or " +
			"parent_name. Optional channel_id/tree_id scopes new roots (MSF). Each node gets a unique " +
			"hierarchical url.path (or optional url_path override). Rejects sibling name dupes and " +
			"within-payload URL collisions before any POST. Server creates level-by-level via " +
			"POST /v3/catalog/trees/categories (chunks of 50). Max 100 nodes, depth 8. " +
			"partial_success when some levels succeed and a later level fails.",
		Tool: mcp.NewTool("catalog_categories_bulk_create",
			mcp.WithDescription(
				"Bulk-create categories as a tree. Prefer nested children in categories_json for "+
					"screenshot/mega-menu recreation. Auto-assigns unique hierarchical url.path values "+
					"so same display names under different parents succeed in one pass. "+
					"One preview covers the whole plan; confirmed=true executes.",
			),
			mcp.WithString("categories_json",
				mcp.Description(`JSON array of category nodes (max 100 total including nested children). `+
					`Each node: {"name":"Men","children":[{"name":"Shoes","children":[{"name":"All Shoes"}]}]}. `+
					`Flat alternative: {"ref":"shoes","name":"Shoes","parent_ref":"men"} with matching {"ref":"men","name":"Men"}. `+
					`Optional per node: url_path (override auto hierarchical slug), parent_id, parent_name `+
					`(existing store parent only), description, is_visible, page_title, meta_description, `+
					`search_keywords, sort_order, default_product_sort.`),
				mcp.Required(),
			),
			mcp.WithNumber("tree_id",
				mcp.Description("Optional: explicit category tree ID for new root categories. Mutually exclusive with channel_id."),
			),
			mcp.WithNumber("channel_id",
				mcp.Description("Optional MSF: resolve tree_id for new roots from this channel ID. Mutually exclusive with tree_id."),
			),
			mcp.WithBoolean("confirmed",
				mcp.Description("Set to true to execute after reviewing the preview."),
			),
		),
		Handler: c.handleBulkCreate,
	})
}

// bulkCreateInputNode is the JSON shape accepted inside categories_json.
type bulkCreateInputNode struct {
	Name               string                `json:"name"`
	Ref                string                `json:"ref"`
	ParentRef          string                `json:"parent_ref"`
	ParentID           *int                  `json:"parent_id"`
	ParentName         string                `json:"parent_name"`
	URLPath            string                `json:"url_path"`
	Description        string                `json:"description"`
	PageTitle          string                `json:"page_title"`
	MetaDescription    string                `json:"meta_description"`
	SearchKeywords     string                `json:"search_keywords"`
	DefaultProductSort string                `json:"default_product_sort"`
	SortOrder          *int                  `json:"sort_order"`
	IsVisible          *bool                 `json:"is_visible"`
	Children           []bulkCreateInputNode `json:"children"`
}

// BulkCategoryCreateNode is a flattened plan node. Exported for tests.
type BulkCategoryCreateNode struct {
	Name               string
	Ref                string
	ParentRef          string
	ResolvedParentID   int
	StoreParentName    string
	Path               string
	URLPath            string
	URLPathOverride    string
	Level              int
	Description        string
	PageTitle          string
	MetaDescription    string
	SearchKeywords     string
	DefaultProductSort string
	SortOrder          int
	HasSortOrder       bool
	IsVisible          *bool
}

// BulkCategoryCreateParams holds parsed bulk-create arguments. Exported for testing.
type BulkCategoryCreateParams struct {
	Nodes     []BulkCategoryCreateNode
	ChannelID int
	TreeID    int
	Confirmed bool
}

func (c *Categories) handleBulkCreate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	params, err := ParseBulkCategoryCreateParams(args)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	if err := c.resolveBulkCreateParents(ctx, params); err != nil {
		return toolError("%s", err.Error()), nil
	}

	rootTreeID, err := c.resolveBulkCreateRootTreeID(ctx, params)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	if params.Confirmed {
		return c.executeBulkCreate(ctx, params, rootTreeID)
	}
	return c.previewBulkCreate(params, rootTreeID)
}

// ParseBulkCategoryCreateParams is exported for unit testing.
func ParseBulkCategoryCreateParams(args map[string]any) (*BulkCategoryCreateParams, error) {
	p := &BulkCategoryCreateParams{}

	raw, ok := args["categories_json"].(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("categories_json is required (a JSON array of category nodes)")
	}

	var roots []bulkCreateInputNode
	if err := json.Unmarshal([]byte(raw), &roots); err != nil {
		return nil, fmt.Errorf("invalid categories_json: %v", err)
	}
	if len(roots) == 0 {
		return nil, fmt.Errorf("categories_json must contain at least one category")
	}

	nodes, err := flattenBulkCreateNodes(roots)
	if err != nil {
		return nil, err
	}
	if len(nodes) > maxBulkCategoryCreateNodes {
		return nil, fmt.Errorf("categories_json: maximum %d categories per call (got %d including nested children)",
			maxBulkCategoryCreateNodes, len(nodes))
	}
	assignBulkCreateURLPaths(nodes)
	if err := validateBulkCreateUniqueness(nodes); err != nil {
		return nil, err
	}
	p.Nodes = nodes

	_, hasTreeID := args["tree_id"]
	_, hasChannelID := args["channel_id"]
	if hasTreeID && hasChannelID {
		return nil, fmt.Errorf("tree_id and channel_id are mutually exclusive; provide one or neither")
	}
	if v, ok := args["tree_id"]; ok {
		f, fOk := v.(float64)
		if !fOk || f <= 0 || f != float64(int(f)) {
			return nil, fmt.Errorf("tree_id must be a positive integer")
		}
		p.TreeID = int(f)
	}
	if v, ok := args["channel_id"]; ok {
		f, fOk := v.(float64)
		if !fOk || f <= 0 || f != float64(int(f)) {
			return nil, fmt.Errorf("channel_id must be a positive integer")
		}
		p.ChannelID = int(f)
	}
	if v, ok := args["confirmed"]; ok {
		b, bOk := v.(bool)
		if bOk {
			p.Confirmed = b
		}
	}

	return p, nil
}

func flattenBulkCreateNodes(roots []bulkCreateInputNode) ([]BulkCategoryCreateNode, error) {
	out := make([]BulkCategoryCreateNode, 0)
	refs := make(map[string]bool)
	autoIdx := 0

	var walk func(node bulkCreateInputNode, parentRef string, pathPrefix string, depth int) error
	walk = func(node bulkCreateInputNode, parentRef string, pathPrefix string, depth int) error {
		if depth > maxBulkCategoryCreateDepth {
			return fmt.Errorf("category tree exceeds BigCommerce max depth of %d", maxBulkCategoryCreateDepth)
		}
		name := strings.TrimSpace(node.Name)
		if name == "" {
			return fmt.Errorf("each category node requires a non-empty name")
		}

		ref := strings.TrimSpace(node.Ref)
		if ref == "" {
			ref = fmt.Sprintf("n%d", autoIdx)
			autoIdx++
		}
		if refs[ref] {
			return fmt.Errorf("duplicate ref %q — refs must be unique within categories_json", ref)
		}
		refs[ref] = true

		parentModes := 0
		storeParentID := 0
		storeParentName := ""
		effectiveParentRef := strings.TrimSpace(node.ParentRef)

		if parentRef != "" {
			// Nested under children[] — parent is the enclosing node.
			if effectiveParentRef != "" || node.ParentID != nil || strings.TrimSpace(node.ParentName) != "" {
				return fmt.Errorf("category %q: nested children inherit their parent; do not set parent_ref, parent_id, or parent_name", name)
			}
			effectiveParentRef = parentRef
		} else {
			if effectiveParentRef != "" {
				parentModes++
			}
			if node.ParentID != nil {
				parentModes++
				if *node.ParentID < 0 {
					return fmt.Errorf("category %q: parent_id must be >= 0", name)
				}
				storeParentID = *node.ParentID
			}
			if strings.TrimSpace(node.ParentName) != "" {
				parentModes++
				storeParentName = strings.TrimSpace(node.ParentName)
			}
			if parentModes > 1 {
				return fmt.Errorf("category %q: parent_id, parent_name, and parent_ref are mutually exclusive", name)
			}
		}

		if node.DefaultProductSort != "" {
			validSorts := map[string]bool{
				"best_selling": true, "price_desc": true, "price_asc": true,
				"avg_customer_review": true, "alpha_asc": true, "alpha_desc": true,
				"featured": true, "newest": true, "use_store_settings": true,
			}
			if !validSorts[node.DefaultProductSort] {
				return fmt.Errorf("category %q: invalid default_product_sort %q", name, node.DefaultProductSort)
			}
		}

		path := name
		if pathPrefix != "" {
			path = pathPrefix + " > " + name
		}

		flat := BulkCategoryCreateNode{
			Name:               name,
			Ref:                ref,
			ParentRef:          effectiveParentRef,
			ResolvedParentID:   storeParentID,
			StoreParentName:    storeParentName,
			Path:               path,
			URLPathOverride:    strings.TrimSpace(node.URLPath),
			Level:              depth,
			Description:        node.Description,
			PageTitle:          node.PageTitle,
			MetaDescription:    node.MetaDescription,
			SearchKeywords:     node.SearchKeywords,
			DefaultProductSort: node.DefaultProductSort,
			IsVisible:          node.IsVisible,
		}
		if node.SortOrder != nil {
			flat.SortOrder = *node.SortOrder
			flat.HasSortOrder = true
		}
		out = append(out, flat)

		for _, child := range node.Children {
			if err := walk(child, ref, path, depth+1); err != nil {
				return err
			}
		}
		return nil
	}

	for _, root := range roots {
		if err := walk(root, "", "", 1); err != nil {
			return nil, err
		}
	}

	// Validate parent_ref targets exist.
	for _, n := range out {
		if n.ParentRef == "" {
			continue
		}
		if !refs[n.ParentRef] {
			return nil, fmt.Errorf("category %q: parent_ref %q does not match any ref in categories_json", n.Name, n.ParentRef)
		}
		if n.ParentRef == n.Ref {
			return nil, fmt.Errorf("category %q: parent_ref cannot reference itself", n.Name)
		}
	}

	// Detect cycles in parent_ref chains and assign levels from chain depth
	// so flat ref/parent_ref lists create parents before children.
	indexByRef := make(map[string]int, len(out))
	for i, n := range out {
		indexByRef[n.Ref] = i
	}
	for i := range out {
		seen := map[string]bool{}
		cur := out[i].Ref
		depth := 1
		for {
			if seen[cur] {
				return nil, fmt.Errorf("category ref cycle detected involving %q", cur)
			}
			seen[cur] = true
			parent := out[indexByRef[cur]].ParentRef
			if parent == "" {
				break
			}
			depth++
			if depth > maxBulkCategoryCreateDepth {
				return nil, fmt.Errorf("category tree exceeds BigCommerce max depth of %d (via parent_ref chain)", maxBulkCategoryCreateDepth)
			}
			cur = parent
		}
		out[i].Level = depth
	}

	// Rebuild display Path from parent_ref chains so flat lists get hierarchical
	// url_path slugs (nested children[] already set Path during walk).
	for i := range out {
		segments := []string{out[i].Name}
		cur := out[i].ParentRef
		guard := 0
		for cur != "" && guard <= maxBulkCategoryCreateDepth {
			guard++
			idx, ok := indexByRef[cur]
			if !ok {
				break
			}
			segments = append([]string{out[idx].Name}, segments...)
			cur = out[idx].ParentRef
		}
		out[i].Path = strings.Join(segments, " > ")
	}

	return out, nil
}

// SlugifyCategorySegment converts a category name segment into a URL slug piece.
// Exported for unit tests.
func SlugifyCategorySegment(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "&", " and ")
	s = bulkCategorySlugNonAlnum.ReplaceAllString(s, "-")
	s = bulkCategorySlugTrim.ReplaceAllString(s, "")
	if s == "" {
		return "category"
	}
	return s
}

// NormalizeCategoryURLPath ensures a storefront path has leading and trailing slashes.
// Exported for unit tests.
func NormalizeCategoryURLPath(raw string) (string, error) {
	p := strings.TrimSpace(raw)
	if p == "" {
		return "", fmt.Errorf("url_path must not be empty")
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if !strings.HasSuffix(p, "/") {
		p = p + "/"
	}
	// Collapse accidental double slashes (but keep single leading/trailing).
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	if p == "/" {
		return "", fmt.Errorf("url_path must include at least one path segment")
	}
	return p, nil
}

// CategoryURLPathFromDisplayPath builds /seg1/seg2/ from a "Seg1 > Seg2" display path.
// Exported for unit tests.
func CategoryURLPathFromDisplayPath(displayPath string) string {
	parts := strings.Split(displayPath, " > ")
	segs := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		segs = append(segs, SlugifyCategorySegment(part))
	}
	if len(segs) == 0 {
		return "/category/"
	}
	return "/" + strings.Join(segs, "/") + "/"
}

func assignBulkCreateURLPaths(nodes []BulkCategoryCreateNode) {
	for i := range nodes {
		if nodes[i].URLPathOverride != "" {
			normalized, err := NormalizeCategoryURLPath(nodes[i].URLPathOverride)
			if err != nil {
				// Leave empty; validateBulkCreateUniqueness will surface a clear error.
				nodes[i].URLPath = ""
				continue
			}
			nodes[i].URLPath = normalized
			continue
		}
		nodes[i].URLPath = CategoryURLPathFromDisplayPath(nodes[i].Path)
	}
}

func validateBulkCreateUniqueness(nodes []BulkCategoryCreateNode) error {
	for i := range nodes {
		if nodes[i].URLPathOverride != "" && nodes[i].URLPath == "" {
			return fmt.Errorf("category %q: invalid url_path %q", nodes[i].Name, nodes[i].URLPathOverride)
		}
	}

	// Sibling display-name uniqueness (BigCommerce rule).
	type siblingKey struct {
		parent string
		name   string
	}
	siblings := make(map[siblingKey]string, len(nodes))
	for _, n := range nodes {
		parentKey := fmt.Sprintf("id:%d", n.ResolvedParentID)
		if n.ParentRef != "" {
			parentKey = "ref:" + n.ParentRef
		} else if n.StoreParentName != "" {
			parentKey = "name:" + strings.ToLower(n.StoreParentName)
		}
		key := siblingKey{parent: parentKey, name: strings.ToLower(n.Name)}
		if prev, ok := siblings[key]; ok {
			return fmt.Errorf("duplicate sibling category name %q under the same parent (refs %q and %q); BigCommerce requires unique names among siblings",
				n.Name, prev, n.Ref)
		}
		siblings[key] = n.Ref
	}

	// Within-payload URL path uniqueness.
	seenURL := make(map[string]string, len(nodes))
	for _, n := range nodes {
		if n.URLPath == "" {
			return fmt.Errorf("category %q: failed to derive url_path", n.Name)
		}
		if prev, ok := seenURL[n.URLPath]; ok {
			return fmt.Errorf("duplicate url_path %q for categories %q and %q — provide distinct url_path overrides or rename so hierarchical slugs differ",
				n.URLPath, prev, n.Name)
		}
		seenURL[n.URLPath] = n.Name
	}
	return nil
}

func (c *Categories) resolveBulkCreateParents(ctx context.Context, params *BulkCategoryCreateParams) error {
	for i := range params.Nodes {
		n := &params.Nodes[i]
		if n.StoreParentName == "" {
			continue
		}
		id, err := c.resolveParentName(ctx, n.StoreParentName)
		if err != nil {
			return fmt.Errorf("category %q: %w", n.Name, err)
		}
		n.ResolvedParentID = id
	}
	return nil
}

func (c *Categories) resolveBulkCreateRootTreeID(ctx context.Context, params *BulkCategoryCreateParams) (int, error) {
	needsRootTree := false
	for _, n := range params.Nodes {
		if n.ParentRef == "" && n.ResolvedParentID == 0 {
			needsRootTree = true
			break
		}
	}
	if !needsRootTree {
		return 0, nil
	}
	if params.TreeID > 0 {
		return params.TreeID, nil
	}
	if params.ChannelID > 0 {
		treeID, err := c.bc.GetTreeIDForChannel(ctx, params.ChannelID)
		if err != nil {
			return 0, fmt.Errorf("failed to resolve tree for channel %d: %v", params.ChannelID, err)
		}
		return treeID, nil
	}
	treeID, err := c.bc.GetDefaultTreeID(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to determine category tree: %v", err)
	}
	return treeID, nil
}

func (c *Categories) previewBulkCreate(params *BulkCategoryCreateParams, rootTreeID int) (*mcp.CallToolResult, error) {
	levels := map[int]int{}
	sample := make([]map[string]any, 0, len(params.Nodes))
	for _, n := range params.Nodes {
		levels[n.Level]++
		row := map[string]any{
			"ref":      n.Ref,
			"name":     n.Name,
			"path":     n.Path,
			"url_path": n.URLPath,
			"level":    n.Level,
		}
		if n.ParentRef != "" {
			row["parent_ref"] = n.ParentRef
		}
		if n.ResolvedParentID > 0 {
			row["parent_id"] = n.ResolvedParentID
			if n.StoreParentName != "" {
				row["parent_name"] = n.StoreParentName
			}
		} else if n.ParentRef == "" {
			row["parent_id"] = 0
			if rootTreeID > 0 {
				row["tree_id"] = rootTreeID
			}
		}
		sample = append(sample, row)
	}

	levelSummary := make([]map[string]any, 0, len(levels))
	maxLevel := 0
	for lvl := range levels {
		if lvl > maxLevel {
			maxLevel = lvl
		}
	}
	for lvl := 1; lvl <= maxLevel; lvl++ {
		if count, ok := levels[lvl]; ok {
			levelSummary = append(levelSummary, map[string]any{"level": lvl, "count": count})
		}
	}

	preview := map[string]any{
		"status":         "preview",
		"message":        "Review the category tree below. Pass confirmed=true with the same parameters to create it level-by-level.",
		"total":          len(params.Nodes),
		"max_depth":      maxLevel,
		"levels":         levelSummary,
		"categories":     sample,
		"api":            "POST /v3/catalog/trees/categories (batched, level-by-level)",
		"chunk_size":     50,
		"partial_policy": "If a later level fails after earlier levels succeed, status is partial_success with created IDs so far.",
	}
	if rootTreeID > 0 {
		preview["root_tree_id"] = rootTreeID
	}
	if params.ChannelID > 0 {
		preview["channel_id"] = params.ChannelID
	}
	return toolJSON(preview)
}

func bulkCreatePayload(n BulkCategoryCreateNode, parentID, rootTreeID int) bigcommerce.CategoryCreate {
	payload := bigcommerce.CategoryCreate{
		Name:               n.Name,
		Description:        n.Description,
		PageTitle:          n.PageTitle,
		MetaDescription:    n.MetaDescription,
		SearchKeywords:     n.SearchKeywords,
		DefaultProductSort: n.DefaultProductSort,
		IsVisible:          n.IsVisible,
	}
	if n.URLPath != "" {
		payload.URL = &bigcommerce.CustomURL{
			Path:         n.URLPath,
			IsCustomized: true,
		}
	}
	if n.HasSortOrder {
		payload.SortOrder = n.SortOrder
	}
	if parentID > 0 {
		payload.ParentID = parentID
	} else {
		payload.TreeID = rootTreeID
	}
	return payload
}

func appendCreatedCategory(created *[]map[string]any, n BulkCategoryCreateNode, cat bigcommerce.Category) {
	*created = append(*created, map[string]any{
		"id":        cat.ID,
		"name":      cat.Name,
		"parent_id": cat.ParentID,
		"tree_id":   cat.TreeID,
		"ref":       n.Ref,
		"path":      n.Path,
		"url_path":  n.URLPath,
		"level":     n.Level,
	})
}

func (c *Categories) executeBulkCreate(ctx context.Context, params *BulkCategoryCreateParams, rootTreeID int) (*mcp.CallToolResult, error) {
	refToID := make(map[string]int, len(params.Nodes))
	created := make([]map[string]any, 0, len(params.Nodes))
	var levelErrors []string

	maxLevel := 0
	for _, n := range params.Nodes {
		if n.Level > maxLevel {
			maxLevel = n.Level
		}
	}

	for level := 1; level <= maxLevel; level++ {
		ready := make([]BulkCategoryCreateNode, 0)
		payloads := make([]bigcommerce.CategoryCreate, 0)
		for _, n := range params.Nodes {
			if n.Level != level {
				continue
			}
			parentID := n.ResolvedParentID
			if n.ParentRef != "" {
				id, ok := refToID[n.ParentRef]
				if !ok {
					levelErrors = append(levelErrors,
						fmt.Sprintf("level %d category %q: parent_ref %q was not created (earlier failure)", level, n.Name, n.ParentRef))
					continue
				}
				parentID = id
			}
			ready = append(ready, n)
			payloads = append(payloads, bulkCreatePayload(n, parentID, rootTreeID))
		}
		if len(ready) == 0 {
			continue
		}

		cats, err := c.bc.CreateCategories(ctx, payloads)
		for i, cat := range cats {
			if i >= len(ready) {
				break
			}
			refToID[ready[i].Ref] = cat.ID
			appendCreatedCategory(&created, ready[i], cat)
		}
		if err != nil {
			levelErrors = append(levelErrors, fmt.Sprintf("level %d: %v", level, err))
			break
		}
		if len(cats) != len(ready) {
			levelErrors = append(levelErrors,
				fmt.Sprintf("level %d: expected %d created categories, got %d", level, len(ready), len(cats)))
			break
		}
	}

	status := "created"
	message := fmt.Sprintf("Created %d categories.", len(created))
	if len(levelErrors) > 0 {
		if len(created) == 0 {
			return toolError("bulk category create failed: %s", strings.Join(levelErrors, "; ")), nil
		}
		status = "partial_success"
		message = fmt.Sprintf("Created %d of %d categories; later levels failed.", len(created), len(params.Nodes))
	} else if len(created) < len(params.Nodes) {
		status = "partial_success"
		message = fmt.Sprintf("Created %d of %d categories (some skipped due to missing parents).", len(created), len(params.Nodes))
	}

	result := map[string]any{
		"status":     status,
		"message":    message,
		"created":    len(created),
		"requested":  len(params.Nodes),
		"categories": created,
	}
	if len(levelErrors) > 0 {
		result["errors"] = levelErrors
	}
	return toolJSON(result)
}
