package flows

// Subflows (spec 009): a flow of kind "subflow" is a reusable piece with one input node (sub_in) and output nodes
// (sub_out, output 1–4). A "subflow" node in another flow runs it: before the flow starts, every subflow node is
// expanded into a copy of the subflow's nodes (ids "<node>.<subflow node>"), with the wires into the subflow node
// leading to its sub_in and the wires from its output k leaving the sub_out nodes of port k. The subflow version is
// pinned in the node (settings.version), so a changed subflow reaches a flow only when that flow is saved and
// deployed again (four eyes and audit apply as for any flow change).

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// MaxSubflowDepth limits subflows inside subflows.
const MaxSubflowDepth = 4

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// SubflowRef describes a subflow version for the node that uses it.
type SubflowRef struct {
	Name     string
	Version  int
	Outputs  int
	Commands bool
}

// SubflowInfo derives the outputs and command use of a subflow spec.
func SubflowInfo(s *Spec) (outputs int, commands bool) {
	for _, n := range s.Nodes {
		if n.Type == "sub_out" && !n.Disabled {
			if p, ok := Num(n.Settings["port"]); ok && int(p) > outputs {
				outputs = int(p)
			}
		}
	}
	return outputs, HasCommands(s)
}

// Loader returns a stored subflow version (version 0 = the current one).
type Loader func(slug string, version int) (*Spec, error)

// Expand replaces the subflow nodes of a spec by copies of the subflows' nodes. Specs without subflow nodes are
// returned unchanged.
func Expand(s *Spec, load Loader) (*Spec, error) {
	return expand(s, load, 0, map[string]bool{})
}

func expand(s *Spec, load Loader, depth int, path map[string]bool) (*Spec, error) {
	has := false
	for _, n := range s.Nodes {
		if n.Type == "subflow" && !n.Disabled {
			has = true
			break
		}
	}
	if !has {
		return s, nil
	}
	if depth >= MaxSubflowDepth {
		return nil, fmt.Errorf("subflows nested deeper than %d", MaxSubflowDepth)
	}
	out := &Spec{Kind: s.Kind}
	type inst struct {
		in   string           // expanded id of sub_in
		outs map[int][]string // port (0-based) -> expanded ids of sub_out
	}
	insts := map[string]inst{}
	for _, n := range s.Nodes {
		if n.Type != "subflow" || n.Disabled {
			out.Nodes = append(out.Nodes, n)
			continue
		}
		slug, _ := n.Settings["flow"].(string)
		ver := 0
		if v, ok := Num(n.Settings["version"]); ok {
			ver = int(v)
		}
		if path[slug] {
			return nil, fmt.Errorf("%s: subflow %s uses itself", n.ID, slug)
		}
		sub, err := load(slug, ver)
		if err != nil {
			return nil, fmt.Errorf("%s: subflow %s: %w", n.ID, slug, err)
		}
		if sub.Kind != "subflow" {
			return nil, fmt.Errorf("%s: %s is not a subflow", n.ID, slug)
		}
		path[slug] = true
		sub, err = expand(sub, load, depth+1, path)
		delete(path, slug)
		if err != nil {
			return nil, err
		}
		it := inst{outs: map[int][]string{}}
		pre := n.ID + "."
		for _, sn := range sub.Nodes {
			c := sn
			c.ID = pre + sn.ID
			c.Settings = cloneSettings(sn.Settings)
			nested := strings.Contains(sn.ID, ".") // inputs/outputs of subflows inside this subflow stay internal
			switch {
			case nested:
			case sn.Type == "sub_in":
				it.in = c.ID
			case sn.Type == "sub_out":
				p := 1
				if v, ok := Num(sn.Settings["port"]); ok {
					p = int(v)
				}
				it.outs[p-1] = append(it.outs[p-1], c.ID)
			}
			out.Nodes = append(out.Nodes, c)
		}
		if it.in == "" {
			return nil, fmt.Errorf("%s: subflow %s has no input", n.ID, slug)
		}
		for _, w := range sub.Wires {
			out.Wires = append(out.Wires, Wire{From: pre + w.From, Port: w.Port, To: pre + w.To})
		}
		insts[n.ID] = it
	}
	for _, w := range s.Wires {
		froms := []Wire{w}
		if it, ok := insts[w.From]; ok {
			froms = nil
			for _, o := range it.outs[w.Port] {
				froms = append(froms, Wire{From: o, Port: 0, To: w.To})
			}
		}
		for _, fw := range froms {
			if it, ok := insts[fw.To]; ok {
				fw.To = it.in
			}
			out.Wires = append(out.Wires, fw)
		}
	}
	return out, nil
}

func cloneSettings(m map[string]any) map[string]any {
	c := make(map[string]any, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}

// ErrNotDeployable is returned for subflows, which run only inside flows.
var ErrNotDeployable = errors.New("a subflow runs inside flows and is not deployed itself")
