package internal

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/H-BF/corlib/pkg/slice"
	"github.com/H-BF/protos/pkg/api/common"
	sgroupsAPI "github.com/H-BF/protos/pkg/api/sgroups"
	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

const (
	// RcLabelRules -
	RcLabelRules = "sg-rules"
	// RcLabelSgFrom -
	RcLabelSgFrom = "sg-from"
	// RcLabelSgTo -
	RcLabelSgTo = "sg-to"
	// RcLabelProto -
	RcLabelProto = "proto"
	// RcLabelPortsFrom -
	RcLabelPortsFrom = "ports-from"
	// RcLabelPortsTo -
	RcLabelPortsTo = "ports-to"
)

// SGroupsRcRules sg-rules resource
func SGroupsRcRules() *schema.Resource {
	return &schema.Resource{
		Description:   fmt.Sprintf("represent SG rules resource in '%s' provider", SGroupsProvider),
		CreateContext: rulesUpsert,
		UpdateContext: rulesUpsert,
		DeleteContext: rulesDelete,
		Schema: map[string]*schema.Schema{
			RcLabelItems: {
				Description: "SG rules list",
				Type:        schema.TypeList,
				Required:    true,
				MinItems:    1,
				Elem: &schema.Resource{
					Description: "SG rule element",
					Schema: map[string]*schema.Schema{
						RcLabelProto: {
							Description: "ip-proto tcp|udp",
							Type:        schema.TypeString,
							Required:    true,
							ValidateDiagFunc: func(i interface{}, p cty.Path) diag.Diagnostics {
								s := i.(string)
								ok := strings.EqualFold(common.Networks_NetIP_TCP.String(), s) ||
									strings.EqualFold(common.Networks_NetIP_UDP.String(), s)
								if ok {
									return nil
								}
								return diag.Diagnostics{{
									Severity:      diag.Error,
									AttributePath: p,
									Summary:       fmt.Sprintf("bad proto: '%s'", s),
								}}
							},
						},
						RcLabelSgFrom: {
							Description: "SG from",
							Type:        schema.TypeString,
							Required:    true,
						},
						RcLabelSgTo: {
							Description: "SG to",
							Type:        schema.TypeString,
							Required:    true,
						},
						RcLabelPortsFrom: {
							Description:      "port ranges from",
							Type:             schema.TypeString,
							Required:         true,
							ValidateDiagFunc: validatePortRanges,
						},
						RcLabelPortsTo: {
							Description:      "port ranges to",
							Type:             schema.TypeString,
							Required:         true,
							ValidateDiagFunc: validatePortRanges,
						},
					},
				},
			},
		},
	}
}

func rulesUpsert(ctx context.Context, rd *schema.ResourceData, i interface{}) diag.Diagnostics {
	items := rd.Get(RcLabelItems).([]interface{})
	var syncRules sgroupsAPI.SyncSGRules
	var keys []string
	for _, it := range items {
		item := it.(map[string]interface{})
		proto := common.Networks_NetIP_Transport_value[strings.ToUpper(item[RcLabelPortsFrom].(string))]
		rule := sgroupsAPI.Rule{
			Transport: common.Networks_NetIP_Transport(proto),
			SgFrom: &sgroupsAPI.SecGroup{
				Name: item[RcLabelSgFrom].(string),
			},
			SgTo: &sgroupsAPI.SecGroup{
				Name: item[RcLabelSgTo].(string),
			},
		}
		portsFrom := item[RcLabelPortsFrom].(string)
		err := parsePorts(portsFrom, func(start, end uint16) error {
			rule.PortsFrom = append(rule.PortsFrom, &common.Networks_NetIP_PortRange{
				From: uint32(start),
				To:   uint32(end),
			})
			return nil
		})
		if err != nil {
			return diag.Diagnostics{{
				Severity: diag.Error,
				Summary:  fmt.Sprintf("ports-from '%s', %s", portsFrom, err.Error()),
			}}
		}
		portsTo := item[RcLabelPortsTo].(string)
		err = parsePorts(portsTo, func(start, end uint16) error {
			rule.PortsTo = append(rule.PortsTo, &common.Networks_NetIP_PortRange{
				From: uint32(start),
				To:   uint32(end),
			})
			return nil
		})
		if err != nil {
			return diag.Diagnostics{{
				Severity: diag.Error,
				Summary:  fmt.Sprintf("ports-to '%s', %s", portsTo, err.Error()),
			}}
		}
		syncRules.Rules = append(syncRules.Rules, &rule)
		keys = append(keys, fmt.Sprintf("%s:%s-%s",
			rule.Transport.String(), rule.SgFrom.Name, rule.SgTo.Name))
	}
	req := sgroupsAPI.SyncReq{
		SyncOp: sgroupsAPI.SyncReq_Upsert,
		Subject: &sgroupsAPI.SyncReq_SgRules{
			SgRules: &syncRules,
		},
	}
	c := i.(SGClient)
	_, err := c.Sync(ctx, &req)
	if err != nil {
		return diag.FromErr(err)
	}
	sort.Strings(keys)
	_ = slice.DedupSlice(&keys, func(i, j int) bool {
		return keys[i] == keys[j]
	})
	rd.SetId(strings.Join(keys, ";"))
	return nil
}

func rulesDelete(ctx context.Context, rd *schema.ResourceData, i interface{}) diag.Diagnostics {
	items := rd.Get(RcLabelItems).([]interface{})
	var syncRules sgroupsAPI.SyncSGRules
	for _, it := range items {
		item := it.(map[string]interface{})
		proto := common.Networks_NetIP_Transport_value[strings.ToUpper(item[RcLabelPortsFrom].(string))]
		rule := sgroupsAPI.Rule{
			Transport: common.Networks_NetIP_Transport(proto),
			SgFrom: &sgroupsAPI.SecGroup{
				Name: item[RcLabelSgFrom].(string),
			},
			SgTo: &sgroupsAPI.SecGroup{
				Name: item[RcLabelSgTo].(string),
			},
		}
		syncRules.Rules = append(syncRules.Rules, &rule)
	}
	req := sgroupsAPI.SyncReq{
		SyncOp: sgroupsAPI.SyncReq_Delete,
		Subject: &sgroupsAPI.SyncReq_SgRules{
			SgRules: &syncRules,
		},
	}
	c := i.(SGClient)
	_, err := c.Sync(ctx, &req)
	return diag.FromErr(err)
}

func validatePortRanges(i interface{}, p cty.Path) diag.Diagnostics {
	src := i.(string)
	if isPortRangesValid(src) {
		return nil
	}
	return diag.Diagnostics{{
		Severity:      diag.Error,
		Summary:       fmt.Sprintf("bad port ranges: '%s'", src),
		AttributePath: p,
	}}
}

func isPortRangesValid(src string) bool {
	var count int
	for len(src) > 0 {
		m := parsePortsRE.FindStringSubmatch(src)
		if len(m) < 4 {
			return false
		}
		src = src[len(m[0]):]
		if len(m[1]) == 0 {
			return false
		}
		if a, b := len(m[2]), len(m[3]); !(a|b == 0 || a*b != 0) {
			return false
		}
		count++
	}
	return count > 0
}

func parsePorts(src string, f func(start, end uint16) error) error {
	var (
		l, r uint64
		err  error
	)
	for len(src) > 0 {
		m := parsePortsRE.FindStringSubmatch(src)
		if len(m) < 4 {
			return errIncorrectPortsSource
		}
		src = src[len(m[0]):]
		if a, b := len(m[2]), len(m[3]); a*b != 0 {
			l, err = strconv.ParseUint(m[2], 10, 16)
			if err == nil {
				r, err = strconv.ParseUint(m[3], 10, 16)
			}
		} else {
			l, err = strconv.ParseUint(m[1], 10, 16)
			r = l
		}
		if err == nil {
			if uint16(r) < uint16(l) {
				return errIncorrectPortsSource
			}
			err = f(uint16(l), uint16(r))
		}
		if err != nil {
			return err
		}
	}
	return nil
}

var (
	errIncorrectPortsSource = fmt.Errorf("incorrect port range(s) source")
	parsePortsRE            = regexp.MustCompile(`^\s*((?:(\d+)\s*-\s*(\d+))|\d+)\s*`)
)