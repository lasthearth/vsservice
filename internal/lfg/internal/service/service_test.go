package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	lfgv1 "github.com/lasthearth/vsservice/gen/lfg/v1"
	"github.com/lasthearth/vsservice/internal/lfg/internal/ierror"
	"github.com/lasthearth/vsservice/internal/lfg/internal/model"
	"github.com/lasthearth/vsservice/internal/lfg/internal/service"
	"github.com/lasthearth/vsservice/internal/lfg/internal/service/sermapper"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// fakeRepo keeps posts in memory and mirrors the callback update: a post
// changes only when updateFn succeeds.
type fakeRepo struct {
	posts       map[string]*model.Post
	open        int64
	countedKind model.Kind
}

func (r *fakeRepo) Create(_ context.Context, p *model.Post) (*model.Post, error) {
	r.posts["new"] = p
	return p, nil
}

func (r *fakeRepo) Get(_ context.Context, id string) (*model.Post, error) {
	if p, ok := r.posts[id]; ok {
		return p, nil
	}
	return nil, ierror.ErrNotFound
}

func (r *fakeRepo) ListOpen(context.Context, time.Time, model.Kind, model.Activity, int) ([]model.Post, error) {
	return nil, nil
}

func (r *fakeRepo) CountOpenByAuthor(_ context.Context, _ string, kind model.Kind, _ time.Time) (int64, error) {
	r.countedKind = kind
	return r.open, nil
}

func (r *fakeRepo) UpdatePost(
	ctx context.Context,
	id string,
	fn func(context.Context, *model.Post) (*model.Post, error),
) (*model.Post, error) {
	p, ok := r.posts[id]
	if !ok {
		return nil, ierror.ErrNotFound
	}
	working := *p
	updated, err := fn(ctx, &working)
	if err != nil {
		return nil, err
	}
	r.posts[id] = updated
	return updated, nil
}

func newService(t *testing.T) (*service.Service, *fakeRepo) {
	t.Helper()
	zc := zap.NewProductionConfig()
	l, err := logger.New(&zc)
	if err != nil {
		t.Fatal(err)
	}
	repo := &fakeRepo{posts: map[string]*model.Post{}}
	return service.New(service.Opts{Logger: l, Repo: repo, Mapper: &sermapper.MapperImpl{}}), repo
}

func as(uid string) context.Context {
	return interceptor.ContextWithUserID(context.Background(), uid)
}

func seedPost(t *testing.T, repo *fakeRepo, slots int32) {
	t.Helper()
	start := time.Now().Add(30 * time.Minute)
	p, err := model.NewPost("author", model.Details{
		Kind:       model.KindSession,
		Activities: []model.Activity{model.ActivityMining},
		Title:      "Идём в шахту",
		StartsAt:   &start,
		Slots:      slots,
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	repo.posts["p1"] = p
}

func TestCreatePost(t *testing.T) {
	svc, _ := newService(t)
	start := time.Now().Add(20 * time.Minute)

	got, err := svc.CreatePost(as("author"), &lfgv1.CreatePostRequest{
		Kind:       lfgv1.PostKind_POST_KIND_SESSION,
		Activities: []lfgv1.Activity{lfgv1.Activity_ACTIVITY_EXPLORING},
		Title:      "Руины на востоке",
		StartsAt:   timestamppb.New(start),
		Slots:      3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetActivities()[0] != lfgv1.Activity_ACTIVITY_EXPLORING || got.GetAuthorId() != "author" || got.GetSlots() != 3 {
		t.Fatalf("unexpected post %+v", got)
	}
	if !got.GetExpiresAt().AsTime().Equal(got.GetStartsAt().AsTime().Add(model.SessionLifetime)) {
		t.Fatalf("expires_at %v", got.GetExpiresAt().AsTime())
	}
}

func TestCreatePostLimit(t *testing.T) {
	svc, repo := newService(t)
	repo.open = model.MaxOpenSessionsPerAuthor

	_, err := svc.CreatePost(as("author"), &lfgv1.CreatePostRequest{
		Kind:       lfgv1.PostKind_POST_KIND_SESSION,
		Activities: []lfgv1.Activity{lfgv1.Activity_ACTIVITY_MINING},
		Title:      "Ещё одна шахта",
		StartsAt:   timestamppb.Now(),
		Slots:      1,
	})
	if !errors.Is(err, ierror.ErrOpenPostLimit) {
		t.Fatalf("want ErrOpenPostLimit, got %v", err)
	}
}

func TestRespondJoinsLeavesAndFills(t *testing.T) {
	svc, repo := newService(t)
	seedPost(t, repo, 1)

	got, err := svc.RespondToPost(as("a"), &lfgv1.RespondToPostRequest{Id: "p1"})
	if err != nil || !got.GetJoined() || len(got.GetPost().GetResponders()) != 1 {
		t.Fatalf("a joins: %+v %v", got, err)
	}

	if _, err := svc.RespondToPost(as("b"), &lfgv1.RespondToPostRequest{Id: "p1"}); !errors.Is(err, ierror.ErrPostFull) {
		t.Fatalf("b on a full post: want ErrPostFull, got %v", err)
	}
	if len(repo.posts["p1"].Responders) != 1 {
		t.Fatal("a refused join changed the stored post")
	}

	got, err = svc.RespondToPost(as("a"), &lfgv1.RespondToPostRequest{Id: "p1"})
	if err != nil || got.GetJoined() || len(got.GetPost().GetResponders()) != 0 {
		t.Fatalf("a leaves: %+v %v", got, err)
	}
}

func TestRespondOwnPost(t *testing.T) {
	svc, repo := newService(t)
	seedPost(t, repo, 2)

	if _, err := svc.RespondToPost(as("author"), &lfgv1.RespondToPostRequest{Id: "p1"}); !errors.Is(err, ierror.ErrOwnPost) {
		t.Fatalf("want ErrOwnPost, got %v", err)
	}
}

func TestClosePostAuthorOnly(t *testing.T) {
	svc, repo := newService(t)
	seedPost(t, repo, 2)

	if _, err := svc.ClosePost(as("stranger"), &lfgv1.ClosePostRequest{Id: "p1"}); !errors.Is(err, ierror.ErrNotAuthor) {
		t.Fatalf("stranger: want ErrNotAuthor, got %v", err)
	}
	got, err := svc.ClosePost(as("author"), &lfgv1.ClosePostRequest{Id: "p1"})
	if err != nil || !got.GetClosed() {
		t.Fatalf("author closes: %+v %v", got, err)
	}
	if _, err := svc.RespondToPost(as("a"), &lfgv1.RespondToPostRequest{Id: "p1"}); !errors.Is(err, ierror.ErrPostClosed) {
		t.Fatalf("respond to closed: want ErrPostClosed, got %v", err)
	}
}

func teammateRequest() *lfgv1.CreatePostRequest {
	return &lfgv1.CreatePostRequest{
		Kind:       lfgv1.PostKind_POST_KIND_TEAMMATE,
		Activities: []lfgv1.Activity{lfgv1.Activity_ACTIVITY_BUILDING, lfgv1.Activity_ACTIVITY_FARMING},
		Title:      "Ищу напарника на долгую игру",
		Slots:      1,
		PlayDays:   []int32{6, 7},
		PlayTimes:  []lfgv1.PlayTime{lfgv1.PlayTime_PLAY_TIME_EVENING},
		Experience: lfgv1.Experience_EXPERIENCE_VETERAN,
		Voice:      true,
		Contact:    "discord: fox",
	}
}

func TestCreateTeammatePost(t *testing.T) {
	svc, repo := newService(t)

	got, err := svc.CreatePost(as("author"), teammateRequest())
	if err != nil {
		t.Fatal(err)
	}
	if repo.countedKind != model.KindTeammate {
		t.Fatalf("limit counted against %q", repo.countedKind)
	}
	if got.GetKind() != lfgv1.PostKind_POST_KIND_TEAMMATE || got.GetStartsAt() != nil {
		t.Fatalf("kind %v starts_at %v", got.GetKind(), got.GetStartsAt())
	}
	if !got.GetHasContact() || got.GetExperience() != lfgv1.Experience_EXPERIENCE_VETERAN || !got.GetVoice() {
		t.Fatalf("teammate fields lost: %+v", got)
	}
	if got.GetPlayTimes()[0] != lfgv1.PlayTime_PLAY_TIME_EVENING || len(got.GetPlayDays()) != 2 {
		t.Fatalf("schedule lost: %+v", got)
	}
	if !got.GetExpiresAt().AsTime().Equal(got.GetBumpedAt().AsTime().Add(model.TeammateLifetime)) {
		t.Fatalf("expires_at %v", got.GetExpiresAt().AsTime())
	}
}

func TestCreateTeammatePostLimitIsOne(t *testing.T) {
	svc, repo := newService(t)
	repo.open = model.MaxOpenTeammatePerAuthor

	if _, err := svc.CreatePost(as("author"), teammateRequest()); !errors.Is(err, ierror.ErrOpenPostLimit) {
		t.Fatalf("want ErrOpenPostLimit, got %v", err)
	}
}

func TestTeammateContactAndRenew(t *testing.T) {
	svc, repo := newService(t)
	if _, err := svc.CreatePost(as("author"), teammateRequest()); err != nil {
		t.Fatal(err)
	}
	repo.posts["p1"] = repo.posts["new"]

	got, err := svc.GetPostContact(as("reader"), &lfgv1.GetPostContactRequest{Id: "p1"})
	if err != nil || got.GetContact() != "discord: fox" {
		t.Fatalf("contact: %+v %v", got, err)
	}
	if _, err := svc.GetPostContact(context.Background(), &lfgv1.GetPostContactRequest{Id: "p1"}); err == nil {
		t.Fatal("a guest must not read the contact")
	}

	if _, err := svc.RenewPost(as("author"), &lfgv1.RenewPostRequest{Id: "p1"}); !errors.Is(err, ierror.ErrRenewTooSoon) {
		t.Fatalf("renew right away: want ErrRenewTooSoon, got %v", err)
	}
	if _, err := svc.RenewPost(as("stranger"), &lfgv1.RenewPostRequest{Id: "p1"}); !errors.Is(err, ierror.ErrNotAuthor) {
		t.Fatalf("stranger: want ErrNotAuthor, got %v", err)
	}

	if _, err := svc.ClosePost(as("author"), &lfgv1.ClosePostRequest{Id: "p1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetPostContact(as("reader"), &lfgv1.GetPostContactRequest{Id: "p1"}); !errors.Is(err, ierror.ErrPostClosed) {
		t.Fatalf("closed: want ErrPostClosed, got %v", err)
	}
}

func TestRenewSession(t *testing.T) {
	svc, repo := newService(t)
	seedPost(t, repo, 2)

	if _, err := svc.RenewPost(as("author"), &lfgv1.RenewPostRequest{Id: "p1"}); !errors.Is(err, ierror.ErrNotRenewable) {
		t.Fatalf("want ErrNotRenewable, got %v", err)
	}
}

func TestListPostsNeedsKind(t *testing.T) {
	svc, _ := newService(t)

	if _, err := svc.ListPosts(context.Background(), &lfgv1.ListPostsRequest{}); err == nil {
		t.Fatal("listing without a kind must fail")
	}
	if _, err := svc.ListPosts(context.Background(), &lfgv1.ListPostsRequest{Kind: lfgv1.PostKind_POST_KIND_TEAMMATE}); err != nil {
		t.Fatal(err)
	}
}
