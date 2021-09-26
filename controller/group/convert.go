package group

import (
	"strings"

	"g.hz.netease.com/horizon/common"
	"g.hz.netease.com/horizon/lib/q"
	"g.hz.netease.com/horizon/pkg/group/models"
)

func formatFullPathAndFullName(groups []*models.Group) (string, string) {
	var fullPath, fullName string
	paths := make([]string, len(groups))
	names := make([]string, len(groups))
	for i, model := range groups {
		paths[i] = model.Path
		names[i] = model.Name
	}

	fullPath = "/" + strings.Join(paths, "/")
	fullName = strings.Join(names, " / ")

	return fullPath, fullName
}

func convertGroupToGChild(group *models.Group, fullName, fullPath, resourceType string) *GChild {
	return &GChild{
		ID:              group.ID,
		Name:            group.Name,
		Path:            group.Path,
		VisibilityLevel: group.VisibilityLevel,
		Description:     group.Description,
		ParentID:        group.ParentID,
		FullName:        fullName,
		FullPath:        fullPath,
		Type:            resourceType,
	}
}

func convertNewGroupToGroup(newGroup *NewGroup) *models.Group {
	return &models.Group{
		Name:            newGroup.Name,
		Path:            newGroup.Path,
		VisibilityLevel: newGroup.VisibilityLevel,
		Description:     newGroup.Description,
		ParentID:        newGroup.ParentID,
	}
}

func convertUpdateGroupToGroup(updateGroup *UpdateGroup) *models.Group {
	return &models.Group{
		Name:            updateGroup.Name,
		Path:            updateGroup.Path,
		VisibilityLevel: updateGroup.VisibilityLevel,
		Description:     updateGroup.Description,
	}
}

/*
assuming we have 3 groups,
group one: {id: 1, name: "a", path: "w"}
group two: {id: 2, name: "b", path: "r"}
group three: {id: 3, name: "c", path: "j"}

after the function executed, we get a map:
{
  1: {
    id: 1,
    name: "a",
    path: "w",
    fullName: "a",
    fullPath: "/w"
  },
  1,2: {
    id: 2,
    name: "b",
    path: "r",
    fullName: "a / b",
    fullPath: "/w/r"
  },
  1,2,3: {
    id: 3,
    name: "c",
    path: "j"
    fullName: "a / b / c",
    fullPath: "/w/r/j"
  },
}
*/
func formatTraversalIDsToGChildMap(groups []*models.Group) map[string]*GChild {
	traversalIDsToGChild := make(map[string]*GChild)
	for _, g := range groups {
		traversalIDs := g.TraversalIDs

		var fullName, fullPath string
		if g.ParentID == common.RootGroupID {
			fullName = g.Name
			fullPath = "/" + g.Path
		} else {
			split := strings.Split(traversalIDs, ",")
			prefixIds := strings.Join(split[:len(split)-1], ",")
			prefixGroup := traversalIDsToGChild[prefixIds]
			fullName = prefixGroup.FullName + " / " + g.Name
			fullPath = prefixGroup.FullPath + "/" + g.Path
		}

		gChild := convertGroupToGChild(g, fullName, fullPath, Type)
		traversalIDsToGChild[traversalIDs] = gChild
	}

	return traversalIDsToGChild
}

func formatListGroupQuery(id uint, pageNumber, pageSize int) *q.Query {
	query := q.New(q.KeyWords{
		ParentID: id,
	})
	query.PageNumber = pageNumber
	query.PageSize = pageSize
	// sort by updated_at desc default，let newer items be in head
	s := q.NewSort("updated_at", true)
	query.Sorts = []*q.Sort{s}

	return query
}
