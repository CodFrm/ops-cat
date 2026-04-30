package command

import (
	"context"
	"errors"
	"testing"

	"github.com/opskat/opskat/internal/model/entity/group_entity"
	"github.com/opskat/opskat/internal/repository/group_repo"

	. "github.com/smartystreets/goconvey/convey"
)

type resolveGroupRepoStub struct {
	groups []*group_entity.Group
}

func (s *resolveGroupRepoStub) Find(_ context.Context, id int64) (*group_entity.Group, error) {
	for _, group := range s.groups {
		if group.ID == id {
			return group, nil
		}
	}
	return nil, errors.New("group not found")
}

func (s *resolveGroupRepoStub) List(_ context.Context) ([]*group_entity.Group, error) {
	return s.groups, nil
}

func (s *resolveGroupRepoStub) Create(_ context.Context, _ *group_entity.Group) error { return nil }
func (s *resolveGroupRepoStub) Update(_ context.Context, _ *group_entity.Group) error { return nil }
func (s *resolveGroupRepoStub) Delete(_ context.Context, _ int64) error               { return nil }
func (s *resolveGroupRepoStub) ReparentChildren(_ context.Context, _, _ int64) error  { return nil }
func (s *resolveGroupRepoStub) UpdateSortOrder(_ context.Context, _ int64, _ int) error {
	return nil
}

func TestBuildGroupPathMap(t *testing.T) {
	Convey("buildGroupPathMap", t, func() {
		origGroup := group_repo.Group()
		defer group_repo.RegisterGroup(origGroup)

		Convey("supports hierarchy deeper than five levels", func() {
			group_repo.RegisterGroup(&resolveGroupRepoStub{groups: []*group_entity.Group{
				{ID: 1, Name: "g1"},
				{ID: 2, Name: "g2", ParentID: 1},
				{ID: 3, Name: "g3", ParentID: 2},
				{ID: 4, Name: "g4", ParentID: 3},
				{ID: 5, Name: "g5", ParentID: 4},
				{ID: 6, Name: "g6", ParentID: 5},
				{ID: 7, Name: "g7", ParentID: 6},
			}})

			paths, err := buildGroupPathMap(context.Background())

			So(err, ShouldBeNil)
			So(paths[7], ShouldEqual, "g1/g2/g3/g4/g5/g6/g7")
		})

		Convey("breaks parent cycles from the current group perspective", func() {
			group_repo.RegisterGroup(&resolveGroupRepoStub{groups: []*group_entity.Group{
				{ID: 1, Name: "A", ParentID: 2},
				{ID: 2, Name: "B", ParentID: 1},
			}})

			paths, err := buildGroupPathMap(context.Background())

			So(err, ShouldBeNil)
			So(paths[1], ShouldEqual, "B/A")
			So(paths[2], ShouldEqual, "A/B")
		})
	})
}
