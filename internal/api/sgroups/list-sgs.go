package sgroups

import (
	"context"

	"github.com/H-BF/sgroups/internal/models/sgroups"
	registry "github.com/H-BF/sgroups/internal/registry/sgroups"

	sg "github.com/H-BF/protos/pkg/api/sgroups"
)

// ListSecurityGroups impl 'sgrpups' service
func (srv *sgService) ListSecurityGroups(ctx context.Context, req *sg.ListSecurityGroupsReq) (resp *sg.ListSecurityGroupsResp, err error) {
	defer func() {
		err = correctError(err)
	}()
	var reader registry.Reader
	if reader, err = srv.registryReader(ctx); err != nil {
		return resp, err
	}
	var scope registry.Scope = registry.NoScope
	if names := req.GetSgNames(); len(names) > 0 {
		scope = registry.SG(names[0], names[1:]...)
	}
	resp = new(sg.ListSecurityGroupsResp)
	err = reader.ListSecurityGroups(ctx, func(group sgroups.SecurityGroup) error {
		resp.Groups = append(resp.Groups,
			&sg.SecGroup{
				Name:     group.Name,
				Networks: group.Networks,
			})
		return nil
	}, scope)
	return resp, err
}