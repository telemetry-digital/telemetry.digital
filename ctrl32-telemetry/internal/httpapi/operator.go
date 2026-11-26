package httpapi

// Operator view (spec 036): the picture tree with group alarms and faceplate types bound to an instance.
//
// A screen names its place in the tree in spec.nav; the alarms of a picture are the alarms needing attention on a
// datastream the picture uses, and a tree node carries those of its whole subtree. A faceplate type (spec.faceplate)
// is drawn for one asset and opened for another by re-binding its tags by datastream key.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/alarmmgmt"
)

const maxNavDepth = 5

var fpPrefixRe = regexp.MustCompile(`^[a-z0-9_]{0,32}$`)

// validateNav normalizes spec.nav: parent (slug of another screen), order 0..9999, label up to 24 characters.
func validateNav(src map[string]any) (map[string]any, error) {
	out := map[string]any{"order": 0}
	if v, ok := src["parent"]; ok && v != nil && v != "" {
		p, _ := v.(string)
		if !dashSlugRe.MatchString(p) {
			return nil, errors.New("nav.parent must be the slug of a screen")
		}
		out["parent"] = p
	}
	if v, ok := src["order"]; ok && v != nil {
		f, ok := numOf(v)
		if !ok || f < 0 || f > 9999 || f != float64(int(f)) {
			return nil, errors.New("nav.order must be a whole number 0..9999")
		}
		out["order"] = int(f)
	}
	if v, ok := src["label"]; ok && v != nil && v != "" {
		l, _ := v.(string)
		l = strings.TrimSpace(l)
		if l == "" || len([]rune(l)) > 24 || strings.ContainsAny(l, "<>\n\r\t") {
			return nil, errors.New("nav.label must be plain text of at most 24 characters")
		}
		out["label"] = l
	}
	return out, nil
}

// validateFaceplate normalizes spec.faceplate: the asset the type was drawn for and the key prefix of its tags.
func validateFaceplate(src map[string]any) (map[string]any, error) {
	a, _ := src["asset"].(string)
	if !uuidRe.MatchString(a) {
		return nil, errors.New("faceplate.asset must be the asset the faceplate was drawn for")
	}
	p, _ := src["prefix"].(string)
	if !fpPrefixRe.MatchString(p) {
		return nil, errors.New("faceplate.prefix must be the beginning of datastream keys (a-z, 0-9, _)")
	}
	out := map[string]any{"asset": a}
	if p != "" {
		out["prefix"] = p
	}
	return out, nil
}

// checkNavParent refuses a parent that does not exist, a cycle and a tree deeper than maxNavDepth.
func (s *Server) checkNavParent(ctx context.Context, org, slug, parent string) error {
	rows, err := s.pool.Query(ctx, `SELECT slug, coalesce(spec->'nav'->>'parent', '') FROM core.dashboard WHERE org_id = $1::uuid AND valid_to IS NULL`, org)
	if err != nil {
		return errors.New("query failed")
	}
	defer rows.Close()
	parents := map[string]string{}
	for rows.Next() {
		var sl, p string
		if err := rows.Scan(&sl, &p); err != nil {
			return errors.New("scan failed")
		}
		parents[sl] = p
	}
	if _, ok := parents[parent]; !ok || parent == slug {
		return fmt.Errorf("nav.parent: no other screen %q", parent)
	}
	depth := 2
	for at := parent; ; depth++ {
		up, ok := parents[at]
		if !ok || up == "" {
			break
		}
		if up == slug {
			return errors.New("nav.parent: the screen would be its own ancestor")
		}
		if depth > maxNavDepth*2 { // a cycle among other screens (possible only with rows written before the check)
			break
		}
		at = up
	}
	if depth > maxNavDepth {
		return fmt.Errorf("nav.parent: the picture tree has at most %d levels", maxNavDepth)
	}
	return nil
}

// ---- tree -----------------------------------------------------------------------------------------

type treeNode struct {
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	Label    string `json:"label"`
	Kind     string `json:"kind"`
	Parent   string `json:"parent"`
	Order    int    `json:"order"`
	Depth    int    `json:"depth"`
	Children int    `json:"children"`
	Unack    int    `json:"unack"`
	Active   int    `json:"active"`
	Color    string `json:"color"`
	Priority int    `json:"priority"`
	tags     []string
}

// alarmMark is one alarm needing attention on a datastream.
type alarmMark struct {
	unack, active bool
	priority      int
	color         string
}

// buildScreenTree orders the nodes depth-first (siblings by order, then name) and gives each the group state of its
// subtree. A parent that is missing, a cycle or a branch deeper than maxNavDepth puts the node at the top level.
func buildScreenTree(nodes []*treeNode, alarms map[string][]alarmMark) []*treeNode {
	by := map[string]*treeNode{}
	for _, n := range nodes {
		by[n.Slug] = n
	}
	for _, n := range nodes {
		if _, ok := by[n.Parent]; !ok || n.Parent == n.Slug {
			n.Parent = ""
		}
	}
	for _, n := range nodes { // cut cycles and branches that are too deep at the node where they are found
		steps := 1
		for at := n; at.Parent != ""; at = by[at.Parent] {
			if steps++; steps > maxNavDepth {
				n.Parent = ""
				break
			}
		}
	}
	kids := map[string][]*treeNode{}
	for _, n := range nodes {
		kids[n.Parent] = append(kids[n.Parent], n)
	}
	for _, k := range kids {
		sort.SliceStable(k, func(i, j int) bool {
			if k[i].Order != k[j].Order {
				return k[i].Order < k[j].Order
			}
			return strings.ToLower(k[i].Name) < strings.ToLower(k[j].Name)
		})
	}
	out := make([]*treeNode, 0, len(nodes))
	var walk func(n *treeNode, depth int) map[string]bool
	walk = func(n *treeNode, depth int) map[string]bool {
		n.Depth = depth
		n.Children = len(kids[n.Slug])
		out = append(out, n)
		set := map[string]bool{}
		for _, t := range n.tags {
			set[t] = true
		}
		for _, c := range kids[n.Slug] {
			for t := range walk(c, depth+1) {
				set[t] = true
			}
		}
		n.Unack, n.Active, n.Color, n.Priority = 0, 0, "", 0
		topUnack := false
		for t := range set {
			for _, a := range alarms[t] {
				if a.unack {
					n.Unack++
				}
				if a.active {
					n.Active++
				}
				// the most important: an unacknowledged one before an acknowledged one, then by class priority
				if n.Priority == 0 || (a.unack && !topUnack) || (a.unack == topUnack && a.priority < n.Priority) {
					n.Priority, n.Color, topUnack = a.priority, a.color, a.unack
				}
			}
		}
		return set
	}
	for _, r := range kids[""] {
		walk(r, 1)
	}
	return out
}

// screenMeta is what the tree needs of one version of a screen; versions never change, so it is cached by row id.
type screenMeta struct {
	kind   string
	inTree bool
	parent string
	order  int
	label  string
	tags   []string
}

var screenMetaCache sync.Map // dashboard row id → *screenMeta

func parseScreenMeta(raw []byte) *screenMeta {
	var sp struct {
		Kind string `json:"kind"`
		Nav  *struct {
			Parent string `json:"parent"`
			Order  int    `json:"order"`
			Label  string `json:"label"`
		} `json:"nav"`
		Widgets []struct {
			Bindings map[string][]string `json:"bindings"`
		} `json:"widgets"`
		Elements []struct {
			Tag string `json:"tag"`
			Dyn []struct {
				Tag string `json:"tag"`
			} `json:"dyn"`
		} `json:"elements"`
	}
	_ = json.Unmarshal(raw, &sp)
	m := &screenMeta{kind: sp.Kind}
	if sp.Nav != nil {
		m.inTree, m.parent, m.order, m.label = true, sp.Nav.Parent, sp.Nav.Order, sp.Nav.Label
	}
	seen := map[string]bool{}
	add := func(t string) {
		if t != "" && !seen[t] && uuidRe.MatchString(t) {
			seen[t] = true
			m.tags = append(m.tags, t)
		}
	}
	for _, e := range sp.Elements {
		add(e.Tag)
		for _, d := range e.Dyn {
			add(d.Tag)
		}
	}
	for _, w := range sp.Widgets {
		for _, t := range w.Bindings["datastreams"] {
			add(t)
		}
	}
	return m
}

// screensTree returns the picture tree of the organization with the group alarm state of every node.
func (s *Server) screensTree(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	rows, err := s.pool.Query(ctx, `SELECT id::text, slug, name, is_default FROM core.dashboard WHERE org_id = $1::uuid AND valid_to IS NULL ORDER BY name`, org)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	type rec struct {
		id, slug, name string
		def            bool
	}
	var recs []rec
	for rows.Next() {
		var x rec
		if err := rows.Scan(&x.id, &x.slug, &x.name, &x.def); err != nil {
			rows.Close()
			writeErr(w, 500, "scan failed")
			return
		}
		recs = append(recs, x)
	}
	rows.Close()
	var nodes []*treeNode
	def := ""
	for _, x := range recs {
		var m *screenMeta
		if v, ok := screenMetaCache.Load(x.id); ok {
			m = v.(*screenMeta)
		} else {
			var raw []byte
			if err := s.pool.QueryRow(ctx, `SELECT spec FROM core.dashboard WHERE id = $1::uuid`, x.id).Scan(&raw); err != nil {
				writeErr(w, 500, "query failed")
				return
			}
			m = parseScreenMeta(raw)
			screenMetaCache.Store(x.id, m)
		}
		if !m.inTree {
			continue
		}
		label := m.label
		if label == "" {
			label = x.name
		}
		nodes = append(nodes, &treeNode{Slug: x.slug, Name: x.name, Label: label, Kind: m.kind, Parent: m.parent, Order: m.order, tags: m.tags})
		if x.def {
			def = x.slug
		}
	}
	alarms := map[string][]alarmMark{}
	if len(nodes) > 0 {
		_ = alarmmgmt.EnsureClasses(ctx, s.pool, org)
		arows, err := s.pool.Query(ctx, `SELECT a.datastream_id::text, `+alarmmgmt.NeedsAck+`, a.cleared_at IS NULL, coalesce(c.priority, 1), coalesce(c.color, '#d63939')
			FROM core.alarm a LEFT JOIN core.alarm_rule r ON r.id = a.rule_id`+alarmmgmt.ClassJoin+`
			WHERE a.org_id = $1::uuid AND a.datastream_id IS NOT NULL AND (a.cleared_at IS NULL OR a.acked_at IS NULL) AND`+alarmAttention, org)
		if err != nil {
			s.log.Error("screen tree alarms", "err", err)
			writeErr(w, 500, "query failed")
			return
		}
		for arows.Next() {
			var ds string
			var m alarmMark
			if err := arows.Scan(&ds, &m.unack, &m.active, &m.priority, &m.color); err != nil {
				arows.Close()
				writeErr(w, 500, "scan failed")
				return
			}
			alarms[ds] = append(alarms[ds], m)
		}
		arows.Close()
	}
	tree := buildScreenTree(nodes, alarms)
	home := ""
	for _, n := range tree {
		if home == "" && n.Parent == "" {
			home = n.Slug
		}
		if n.Slug == def {
			home = def
			break
		}
	}
	if tree == nil {
		tree = []*treeNode{}
	}
	writeJSON(w, 200, map[string]any{"home": home, "nodes": tree})
}

// ---- faceplate instances --------------------------------------------------------------------------

// rebindSpec turns a faceplate type into an instance, in place.
//
//	ds      datastream of the example → its counterpart of the instance ("" = none: the tag is unbound)
//	dev     device of the example → the device of the instance ("" = none: its commands are removed, so a faceplate
//	        of one motor can never command the motor it was drawn for)
//	p0, p1  key prefix of the type and of the instance (command keys)
func rebindSpec(spec map[string]any, ds, dev map[string]string, p0, p1 string) {
	key := func(k string) string {
		if p0 != p1 && strings.HasPrefix(k, p0) {
			return p1 + k[len(p0):]
		}
		return k
	}
	elems, _ := spec["elements"].([]any)
	for _, x := range elems {
		e, ok := x.(map[string]any)
		if !ok {
			continue
		}
		if t, _ := e["tag"].(string); t != "" {
			if n, ok := ds[t]; ok {
				if n == "" {
					delete(e, "tag")
				} else {
					e["tag"] = n
				}
			}
		}
		if dyn, ok := e["dyn"].([]any); ok {
			kept := make([]any, 0, len(dyn))
			for _, dx := range dyn {
				if d, ok := dx.(map[string]any); ok {
					if t, _ := d["tag"].(string); t != "" {
						if n, ok := ds[t]; ok {
							if n == "" {
								continue
							}
							d["tag"] = n
						}
					}
				}
				kept = append(kept, dx)
			}
			e["dyn"] = kept
		}
		d, _ := e["device"].(string)
		n, mine := dev[d]
		if d == "" || !mine {
			continue
		}
		on, _ := e["on"].([]any)
		if n == "" {
			delete(e, "device")
			delete(e, "key")
			kept := make([]any, 0, len(on))
			for _, ox := range on {
				if o, ok := ox.(map[string]any); ok && o["act"] == "command" {
					continue
				}
				kept = append(kept, ox)
			}
			if on != nil {
				e["on"] = kept
			}
			continue
		}
		e["device"] = n
		if k, _ := e["key"].(string); k != "" {
			e["key"] = key(k)
		}
		for _, ox := range on {
			if o, ok := ox.(map[string]any); ok && o["act"] == "command" {
				if k, _ := o["key"].(string); k != "" {
					o["key"] = key(k)
				}
			}
		}
	}
	widgets, _ := spec["widgets"].([]any)
	for _, x := range widgets {
		wd, ok := x.(map[string]any)
		if !ok {
			continue
		}
		b, ok := wd["bindings"].(map[string]any)
		if !ok {
			continue
		}
		for name, m := range map[string]map[string]string{"datastreams": ds, "devices": dev} {
			list, ok := b[name].([]any)
			if !ok {
				continue
			}
			kept := make([]any, 0, len(list))
			for _, v := range list {
				id, _ := v.(string)
				if n, ok := m[id]; ok {
					if n == "" {
						continue
					}
					v = n
				}
				kept = append(kept, v)
			}
			b[name] = kept
		}
	}
}

// specTags lists the datastreams a decoded spec refers to.
func specTags(spec map[string]any) []string {
	seen := map[string]bool{}
	var out []string
	add := func(v any) {
		if t, _ := v.(string); uuidRe.MatchString(t) && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	elems, _ := spec["elements"].([]any)
	for _, x := range elems {
		e, _ := x.(map[string]any)
		add(e["tag"])
		dyn, _ := e["dyn"].([]any)
		for _, dx := range dyn {
			d, _ := dx.(map[string]any)
			add(d["tag"])
		}
	}
	widgets, _ := spec["widgets"].([]any)
	for _, x := range widgets {
		wd, _ := x.(map[string]any)
		b, _ := wd["bindings"].(map[string]any)
		list, _ := b["datastreams"].([]any)
		for _, v := range list {
			add(v)
		}
	}
	return out
}

// dashboardInstance returns a faceplate type bound to another asset and/or key prefix.
func (s *Server) dashboardInstance(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	slug := r.PathValue("slug")
	var name string
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT name, spec FROM core.dashboard WHERE org_id = $1::uuid AND valid_to IS NULL AND slug = $2`, org, slug).Scan(&name, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, 404, "dashboard not found")
		return
	}
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	var spec map[string]any
	if err := json.Unmarshal(raw, &spec); err != nil {
		writeErr(w, 500, "spec unreadable")
		return
	}
	fp, _ := spec["faceplate"].(map[string]any)
	a0, _ := fp["asset"].(string)
	p0, _ := fp["prefix"].(string)
	if a0 == "" {
		writeErr(w, 400, "the screen is not a faceplate type")
		return
	}
	q := r.URL.Query()
	asset, tag, p1 := q.Get("asset"), q.Get("tag"), p0
	if q.Has("prefix") {
		p1 = q.Get("prefix")
	}
	if !fpPrefixRe.MatchString(p1) || (asset != "" && !uuidRe.MatchString(asset)) || (tag != "" && !uuidRe.MatchString(tag)) {
		writeErr(w, 400, "asset and tag must be ids, prefix the beginning of datastream keys")
		return
	}
	if asset == "" && tag != "" {
		if err := s.pool.QueryRow(ctx, `SELECT asset_id::text FROM core.datastream WHERE id = $1::uuid AND org_id = $2::uuid`, tag, org).Scan(&asset); err != nil {
			writeErr(w, 404, "datastream not found")
			return
		}
	}
	if asset == "" {
		asset = a0
	}
	var assetName string
	if err := s.pool.QueryRow(ctx, `SELECT name FROM core.asset WHERE id = $1::uuid AND org_id = $2::uuid`, asset, org).Scan(&assetName); err != nil {
		writeErr(w, 404, "asset not found")
		return
	}
	// the tags of the example, their keys, and the datastreams of the instance with the corresponding keys
	ds := map[string]string{}
	want := map[string]string{} // key of the instance → datastream of the example
	rows, err := s.pool.Query(ctx, `SELECT id::text, key FROM core.datastream WHERE org_id = $1::uuid AND asset_id = $2::uuid AND id = ANY($3::uuid[])`, org, a0, specTags(spec))
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	var keys []string
	for rows.Next() {
		var id, k string
		if err := rows.Scan(&id, &k); err != nil {
			rows.Close()
			writeErr(w, 500, "scan failed")
			return
		}
		if strings.HasPrefix(k, p0) {
			ds[id] = ""
			nk := p1 + k[len(p0):]
			want[nk] = id
			keys = append(keys, nk)
		}
	}
	rows.Close()
	rows, err = s.pool.Query(ctx, `SELECT id::text, key FROM core.datastream WHERE org_id = $1::uuid AND asset_id = $2::uuid AND key = ANY($3::text[])`, org, asset, keys)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	var both []string
	for rows.Next() {
		var id, k string
		if err := rows.Scan(&id, &k); err != nil {
			rows.Close()
			writeErr(w, 500, "scan failed")
			return
		}
		ds[want[k]] = id
		both = append(both, want[k], id)
	}
	rows.Close()
	// devices: the device behind a tag of the example → the device behind its counterpart
	devOf := map[string]string{}
	rows, err = s.pool.Query(ctx, `SELECT to_id::text, from_id::text FROM core.assignment WHERE org_id = $1::uuid AND kind = 'device_datastream' AND valid_to IS NULL AND to_id = ANY($2::uuid[])`, org, append(both, mapKeys(ds)...))
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	for rows.Next() {
		var d, dv string
		if err := rows.Scan(&d, &dv); err != nil {
			rows.Close()
			writeErr(w, 500, "scan failed")
			return
		}
		devOf[d] = dv
	}
	rows.Close()
	dev := map[string]string{}
	for old, n := range ds {
		d0 := devOf[old]
		if d0 == "" {
			continue
		}
		d1 := ""
		if n != "" {
			d1 = devOf[n]
		}
		if prev, seen := dev[d0]; !seen {
			dev[d0] = d1
		} else if prev != d1 && d1 != "" { // one device of the example, two of the instance: no safe target
			if prev == "" {
				dev[d0] = d1
			} else {
				dev[d0] = "\x00"
			}
		}
	}
	for d, n := range dev {
		if n == "\x00" {
			dev[d] = ""
		}
	}
	rebindSpec(spec, ds, dev, p0, p1)
	bound := 0
	for _, n := range ds {
		if n != "" {
			bound++
		}
	}
	writeJSON(w, 200, map[string]any{"slug": slug, "name": name, "spec": spec,
		"instance": map[string]any{"asset": asset, "name": assetName, "prefix": p1, "tags": len(ds), "bound": bound}})
}

func mapKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
