package opcua

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/ua"
)

// Browsing the address space and endpoint discovery, so operators pick nodes from the server's tree instead of
// typing node ids (as in FUXA and other SCADA tools). Both connect, answer and disconnect; nothing is stored.

// BrowseLimit caps the children returned for one node.
const BrowseLimit = 2000

// Item is one child of a browsed node.
type Item struct {
	NodeID      string `json:"node_id"`
	BrowseName  string `json:"browse_name"`
	DisplayName string `json:"display_name"`
	NodeClass   string `json:"node_class"` // Object | Variable | Method | View | ObjectType | VariableType | ...
	// Variables only:
	DataType string `json:"data_type,omitempty"`
	Readable bool   `json:"readable,omitempty"`
	Writable bool   `json:"writable,omitempty"`
	Value    any    `json:"value,omitempty"`
	Problem  string `json:"problem,omitempty"` // why the current value cannot be used
	// HasChildren is true for objects, folders and views (variables may have properties; they are browsable too).
	HasChildren bool `json:"has_children"`
}

// BrowseResult is the answer to one browse call.
type BrowseResult struct {
	NodeID     string   `json:"node_id"`
	Items      []Item   `json:"items"`
	Truncated  bool     `json:"truncated,omitempty"`
	Namespaces []string `json:"namespaces,omitempty"`
	Thumbprint string   `json:"server_thumbprint,omitempty"`
}

// Browse lists the children of a node (default: the Objects folder, i=85) with the type, access and current value
// of variables.
func (m *Manager) Browse(ctx context.Context, st Settings, nodeID string) (*BrowseResult, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		nodeID = "i=85"
	}
	parent, err := ua.ParseNodeID(nodeID)
	if err != nil || !nodeIDRe.MatchString(nodeID) {
		return nil, fmt.Errorf("invalid node id %q", nodeID)
	}
	c, thumb, err := m.connect(ctx, st)
	if err != nil {
		return nil, err
	}
	defer c.Close(context.Background())
	bctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	refs, truncated, err := browseAll(bctx, c, parent)
	if err != nil {
		return nil, err
	}
	out := &BrowseResult{NodeID: nodeID, Truncated: truncated, Thumbprint: thumb, Namespaces: c.Namespaces()}
	var vars []int
	for _, r := range refs {
		if r.NodeID == nil || r.NodeID.NodeID == nil {
			continue
		}
		it := Item{NodeID: r.NodeID.NodeID.String(), NodeClass: strings.TrimPrefix(r.NodeClass.String(), "NodeClass")}
		if r.BrowseName != nil {
			it.BrowseName = r.BrowseName.Name
		}
		if r.DisplayName != nil {
			it.DisplayName = r.DisplayName.Text
		}
		if it.DisplayName == "" {
			it.DisplayName = it.BrowseName
		}
		switch r.NodeClass {
		case ua.NodeClassObject, ua.NodeClassView, ua.NodeClassVariable:
			it.HasChildren = true
		}
		if r.NodeClass == ua.NodeClassVariable {
			vars = append(vars, len(out.Items))
		}
		out.Items = append(out.Items, it)
	}
	if len(vars) > 0 {
		describe(bctx, c, out.Items, vars)
	}
	sort.SliceStable(out.Items, func(i, j int) bool {
		a, b := out.Items[i], out.Items[j]
		if (a.NodeClass == "Variable") != (b.NodeClass == "Variable") {
			return a.NodeClass != "Variable" // folders and objects first
		}
		return strings.ToLower(a.DisplayName) < strings.ToLower(b.DisplayName)
	})
	return out, nil
}

func browseAll(ctx context.Context, c *opcua.Client, parent *ua.NodeID) ([]*ua.ReferenceDescription, bool, error) {
	resp, err := c.Browse(ctx, &ua.BrowseRequest{
		View:                          &ua.ViewDescription{ViewID: ua.NewTwoByteNodeID(0)},
		RequestedMaxReferencesPerNode: 500,
		NodesToBrowse: []*ua.BrowseDescription{{
			NodeID: parent, BrowseDirection: ua.BrowseDirectionForward,
			ReferenceTypeID: ua.NewNumericNodeID(0, id.HierarchicalReferences), IncludeSubtypes: true,
			NodeClassMask: uint32(ua.NodeClassObject | ua.NodeClassVariable | ua.NodeClassView | ua.NodeClassMethod),
			ResultMask:    uint32(ua.BrowseResultMaskAll),
		}},
	})
	if err != nil {
		return nil, false, fmt.Errorf("browse: %w", err)
	}
	if len(resp.Results) != 1 {
		return nil, false, errors.New("browse: empty answer")
	}
	res := resp.Results[0]
	if res.StatusCode != ua.StatusOK {
		return nil, false, fmt.Errorf("browse: %s", res.StatusCode.Error())
	}
	refs := res.References
	cp := res.ContinuationPoint
	for len(cp) > 0 {
		if len(refs) >= BrowseLimit {
			_, _ = c.BrowseNext(ctx, &ua.BrowseNextRequest{ReleaseContinuationPoints: true, ContinuationPoints: [][]byte{cp}})
			return refs[:BrowseLimit], true, nil
		}
		next, err := c.BrowseNext(ctx, &ua.BrowseNextRequest{ContinuationPoints: [][]byte{cp}})
		if err != nil || len(next.Results) != 1 || next.Results[0].StatusCode != ua.StatusOK {
			return refs, true, nil
		}
		refs = append(refs, next.Results[0].References...)
		cp = next.Results[0].ContinuationPoint
	}
	if len(refs) > BrowseLimit {
		return refs[:BrowseLimit], true, nil
	}
	return refs, false, nil
}

// describe reads data type, user access level and value of the variables at the given positions.
func describe(ctx context.Context, c *opcua.Client, items []Item, idx []int) {
	const per = 3
	for from := 0; from < len(idx); from += readLimit / per {
		to := min(from+readLimit/per, len(idx))
		var ids []*ua.ReadValueID
		for _, i := range idx[from:to] {
			nid, err := ua.ParseNodeID(items[i].NodeID)
			if err != nil {
				nid = ua.NewNumericNodeID(0, 0)
			}
			ids = append(ids,
				&ua.ReadValueID{NodeID: nid, AttributeID: ua.AttributeIDDataType},
				&ua.ReadValueID{NodeID: nid, AttributeID: ua.AttributeIDUserAccessLevel},
				&ua.ReadValueID{NodeID: nid, AttributeID: ua.AttributeIDValue})
		}
		resp, err := c.Read(ctx, &ua.ReadRequest{NodesToRead: ids, TimestampsToReturn: ua.TimestampsToReturnNeither})
		if err != nil || len(resp.Results) != len(ids) {
			return
		}
		for k, i := range idx[from:to] {
			dt, acc, val := resp.Results[k*per], resp.Results[k*per+1], resp.Results[k*per+2]
			it := &items[i]
			if dt.Status == ua.StatusOK && dt.Value != nil {
				if nid, ok := dt.Value.Value().(*ua.NodeID); ok {
					it.DataType = dataTypeName(nid)
				}
			}
			if acc.Status == ua.StatusOK && acc.Value != nil {
				if a, ok := acc.Value.Value().(uint8); ok {
					it.Readable = a&uint8(ua.AccessLevelTypeCurrentRead) != 0
					it.Writable = a&uint8(ua.AccessLevelTypeCurrentWrite) != 0
				}
			} else {
				it.Readable = val.Status == ua.StatusOK
			}
			v, problem := value(val, Node{})
			it.Value, it.Problem = v, problem
		}
	}
}

var dataTypes = map[uint32]string{
	1: "Boolean", 2: "SByte", 3: "Byte", 4: "Int16", 5: "UInt16", 6: "Int32", 7: "UInt32", 8: "Int64", 9: "UInt64",
	10: "Float", 11: "Double", 12: "String", 13: "DateTime", 14: "Guid", 15: "ByteString", 16: "XmlElement",
	17: "NodeId", 21: "LocalizedText", 22: "Structure", 24: "BaseDataType", 26: "Number", 27: "Integer",
	28: "UInteger", 29: "Enumeration", 290: "Duration", 294: "UtcTime",
}

func dataTypeName(n *ua.NodeID) string {
	if n.Namespace() == 0 && n.Type() != ua.NodeIDTypeString {
		if name, ok := dataTypes[n.IntID()]; ok {
			return name
		}
	}
	return n.String()
}

// Endpoint is one way to connect that a server offers.
type Endpoint struct {
	URL            string   `json:"url"`
	SecurityPolicy string   `json:"security_policy"`
	SecurityMode   string   `json:"security_mode"`
	UserTokens     []string `json:"user_tokens"` // anonymous | username | certificate | issued
	Thumbprint     string   `json:"server_thumbprint,omitempty"`
	ApplicationURI string   `json:"application_uri,omitempty"`
	ProductURI     string   `json:"product_uri,omitempty"`
	ServerName     string   `json:"server_name,omitempty"`
	SecurityLevel  uint8    `json:"security_level"`
	Supported      bool     `json:"supported"` // this client can use the policy
}

// Endpoints asks the server which security policies, modes and logins it accepts (GetEndpoints needs no session).
func (m *Manager) Endpoints(ctx context.Context, endpoint string) ([]Endpoint, error) {
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	eps, err := opcua.GetEndpoints(cctx, endpoint)
	if err != nil {
		return nil, fmt.Errorf("endpoints: %w", err)
	}
	out := make([]Endpoint, 0, len(eps))
	for _, ep := range eps {
		e := Endpoint{URL: ep.EndpointURL, SecurityPolicy: strings.TrimPrefix(ep.SecurityPolicyURI, ua.SecurityPolicyURIPrefix),
			SecurityMode: strings.TrimPrefix(ep.SecurityMode.String(), "MessageSecurityMode"), Thumbprint: Thumbprint(ep.ServerCertificate), SecurityLevel: ep.SecurityLevel}
		if ep.Server != nil {
			e.ApplicationURI, e.ProductURI = ep.Server.ApplicationURI, ep.Server.ProductURI
			if ep.Server.ApplicationName != nil {
				e.ServerName = ep.Server.ApplicationName.Text
			}
		}
		for _, t := range ep.UserIdentityTokens {
			switch t.TokenType {
			case ua.UserTokenTypeAnonymous:
				e.UserTokens = append(e.UserTokens, "anonymous")
			case ua.UserTokenTypeUserName:
				e.UserTokens = append(e.UserTokens, "username")
			case ua.UserTokenTypeCertificate:
				e.UserTokens = append(e.UserTokens, "certificate")
			case ua.UserTokenTypeIssuedToken:
				e.UserTokens = append(e.UserTokens, "issued")
			}
		}
		switch e.SecurityPolicy {
		case "None", "Basic256Sha256", "Basic256", "Basic128Rsa15", "Aes128_Sha256_RsaOaep", "Aes256_Sha256_RsaPss":
			e.Supported = true
		}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].SecurityLevel > out[j].SecurityLevel })
	return out, nil
}
