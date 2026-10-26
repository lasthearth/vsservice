package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	userv1 "github.com/lasthearth/vsservice/gen/user/v1"
	"github.com/lasthearth/vsservice/internal/notification/notificationuc"
	pkgierror "github.com/lasthearth/vsservice/internal/pkg/ierror"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	domainerr "github.com/lasthearth/vsservice/internal/player/internal/ierror"
	"github.com/lasthearth/vsservice/internal/player/internal/model"
	"github.com/lasthearth/vsservice/internal/player/internal/model/verification"
	"github.com/lasthearth/vsservice/internal/player/internal/service/player/sermapper"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// playerState is the stored state of one player the fake repository keeps. It
// is a plain struct so a test can change it without writing model fields.
type playerState struct {
	id         string
	gameName   string
	previous   string
	lastChange time.Time
	status     verification.Status
	about      string
	aboutAt    *time.Time
}

// write records one update the service asked the repository for.
type write struct {
	userId string
	update model.PlayerUpdate
}

// nickWrite records one nickname change the service asked the repository for.
type nickWrite struct {
	userId   string
	old      string
	new      string
	lastSeen time.Time
}

type fakeRepo struct {
	players    map[string]*playerState
	writes     []write
	nickWrites []nickWrite
	page       *model.PlayerListPage
	listErr    error

	gotFilter model.PlayerListFilter
	gotPage   int
	gotSize   int
	gotNow    time.Time
}

func (r *fakeRepo) GetUserById(_ context.Context, id string) (*model.Player, error) {
	p, ok := r.players[id]
	if !ok {
		return nil, domainerr.ErrNotFound
	}

	return &model.Player{
		UserId:                p.id,
		UserName:              p.gameName,
		UserGameName:          p.gameName,
		PreviousNickname:      p.previous,
		LastNicknameChangedAt: p.lastChange,
		About:                 p.about,
		AboutUpdatedAt:        p.aboutAt,
		Verification:          verification.Verification{Status: p.status},
	}, nil
}

func (r *fakeRepo) GetUsersByIds(context.Context, []string) ([]model.Player, error) {
	return nil, nil
}

func (r *fakeRepo) SearchUsers(context.Context, string) ([]model.Player, error) {
	return nil, nil
}

// UpdatePlayerNickname applies the rename the way the real repository does and
// records what it was asked for.
func (r *fakeRepo) UpdatePlayerNickname(
	_ context.Context,
	uid, newNickname, previousNickname string,
	lastChangedAt time.Time,
) error {
	r.nickWrites = append(r.nickWrites, nickWrite{
		userId: uid, old: previousNickname, new: newNickname, lastSeen: lastChangedAt,
	})

	p, ok := r.players[uid]
	if !ok {
		return nil
	}

	p.previous = previousNickname
	p.gameName = newNickname
	p.lastChange = lastChangedAt

	return nil
}

// UpdateById applies the write the way the real repository does: a pointer to an
// empty description means "remove it", not "store an empty string".
func (r *fakeRepo) UpdateById(_ context.Context, uid string, p model.PlayerUpdate) error {
	r.writes = append(r.writes, write{userId: uid, update: p})

	p1, ok := r.players[uid]
	if !ok {
		return nil
	}

	if p.ClearNicknameCooldown {
		p1.lastChange = time.Time{}
	}

	if p.About == nil {
		return nil
	}

	if *p.About == "" {
		p1.about = ""
		p1.aboutAt = nil
		return nil
	}

	p1.about = *p.About
	p1.aboutAt = p.AboutUpdatedAt

	return nil
}

func (r *fakeRepo) ListPlayers(
	_ context.Context,
	filter model.PlayerListFilter,
	page, size int,
	now time.Time,
) (*model.PlayerListPage, error) {
	r.gotFilter, r.gotPage, r.gotSize, r.gotNow = filter, page, size, now
	if r.listErr != nil {
		return nil, r.listErr
	}
	if r.page != nil {
		return r.page, nil
	}

	return &model.PlayerListPage{Page: int32(page), PageSize: int32(size)}, nil
}

// fakeNotifier records the notification the service asked for.
type fakeNotifier struct {
	title   string
	message string
	calls   int
	err     error
}

func (n *fakeNotifier) CreateNotification(
	_ context.Context,
	title, message string,
	_ ...notificationuc.NotificationOpts,
) error {
	n.calls++
	n.title, n.message = title, message
	return n.err
}

func newService(t *testing.T) (*Service, *fakeRepo, *fakeNotifier) {
	t.Helper()

	zc := zap.NewProductionConfig()
	l, err := logger.New(&zc)
	if err != nil {
		t.Fatal(err)
	}

	repo := &fakeRepo{players: map[string]*playerState{}}
	notices := &fakeNotifier{}

	// Storage is not reached by these rpcs. The SSO repository is stubbed
	// because ChangeNickname tells the admins about a self-service change in a
	// background goroutine, and a nil repository there would panic the test
	// process instead of failing one assertion.
	svc := New(Opts{
		DbRepo:  repo,
		SsoRepo: &fakeSso{},
		Mapper:  &sermapper.MapperImpl{},
		Logger:  l,
	})
	svc.cnuc = notices

	return svc, repo, notices
}

// fakeSso stands in for Logto. It keeps no admins, so a self-service change
// notifies nobody and leaves the fake notifier to the admin tests.
type fakeSso struct {
	admins []string
}

func (s *fakeSso) UpdateUserAvatar(context.Context, string, string) error { return nil }

func (s *fakeSso) GetAdminUsers(context.Context) ([]string, error) { return s.admins, nil }

func as(uid string) context.Context {
	return interceptor.ContextWithUserID(context.Background(), uid)
}

// domainCode returns the status code a domain error carries. The service
// returns ierror values; the interceptor maps them onto a gRPC status.
func domainCode(t *testing.T, err error) codes.Code {
	t.Helper()

	if err == nil {
		t.Fatal("expected an error, got nil")
	}

	var de *pkgierror.DomainError
	if !errors.As(err, &de) {
		t.Fatalf("error %v is not a domain error", err)
	}

	return de.Code
}

func statusCode(t *testing.T, err error) codes.Code {
	t.Helper()

	if err == nil {
		t.Fatal("expected an error, got nil")
	}

	return status.Code(err)
}

func TestUpdateMyAboutWritesOnlyTheCaller(t *testing.T) {
	svc, repo, _ := newService(t)
	repo.players["u1"] = &playerState{id: "u1", gameName: "Ветер", status: verification.VerificationStatusVerified}
	repo.players["u2"] = &playerState{id: "u2", gameName: "Дождь", status: verification.VerificationStatusVerified, about: "чужое"}

	got, err := svc.UpdateMyAbout(as("u1"), &userv1.UpdateMyAboutRequest{About: "  О себе: лес  "})
	if err != nil {
		t.Fatalf("UpdateMyAbout: %v", err)
	}

	if got.GetAbout() != "О себе: лес" {
		t.Errorf("about = %q, want the normalized text", got.GetAbout())
	}
	if got.GetAboutUpdatedAt() == nil {
		t.Error("about_updated_at is unset, want the write time")
	}
	if len(repo.writes) != 1 || repo.writes[0].userId != "u1" {
		t.Fatalf("writes = %v, want exactly one for u1", repo.writes)
	}
	if repo.players["u2"].about != "чужое" {
		t.Errorf("another player's description became %q", repo.players["u2"].about)
	}
}

func TestUpdateMyAboutRefusesAPlayerThatIsNotVerified(t *testing.T) {
	cases := []struct {
		name   string
		status verification.Status
	}{
		{"pending", verification.VerificationStatusPending},
		{"rejected", verification.VerificationStatusRejected},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, _ := newService(t)
			repo.players["u1"] = &playerState{id: "u1", status: tc.status}

			_, err := svc.UpdateMyAbout(as("u1"), &userv1.UpdateMyAboutRequest{About: "текст"})

			if code := domainCode(t, err); code != codes.FailedPrecondition {
				t.Errorf("code = %v, want FailedPrecondition", code)
			}
			if len(repo.writes) != 0 {
				t.Errorf("a refused write reached the repository: %v", repo.writes)
			}
		})
	}
}

func TestUpdateMyAboutRefusesAnUnknownPlayer(t *testing.T) {
	svc, _, _ := newService(t)

	_, err := svc.UpdateMyAbout(as("nobody"), &userv1.UpdateMyAboutRequest{About: "текст"})

	if code := domainCode(t, err); code != codes.FailedPrecondition {
		t.Errorf("code = %v, want FailedPrecondition", code)
	}
}

func TestUpdateMyAboutRejectsTextOverTheLimit(t *testing.T) {
	svc, repo, _ := newService(t)
	repo.players["u1"] = &playerState{id: "u1", status: verification.VerificationStatusVerified}

	_, err := svc.UpdateMyAbout(as("u1"), &userv1.UpdateMyAboutRequest{
		About: strings.Repeat("я", model.MaxAboutRunes+1),
	})

	if !errors.Is(err, model.ErrAboutTooLong) {
		t.Fatalf("err = %v, want ErrAboutTooLong", err)
	}
	if len(repo.writes) != 0 {
		t.Errorf("an invalid write reached the repository: %v", repo.writes)
	}
}

func TestUpdateMyAboutRemovesTheDescriptionOnEmptyText(t *testing.T) {
	svc, repo, _ := newService(t)
	at := time.Now().Add(-time.Hour)
	repo.players["u1"] = &playerState{
		id: "u1", status: verification.VerificationStatusVerified, about: "старое", aboutAt: &at,
	}

	got, err := svc.UpdateMyAbout(as("u1"), &userv1.UpdateMyAboutRequest{About: "   "})
	if err != nil {
		t.Fatalf("UpdateMyAbout: %v", err)
	}

	if got.GetAbout() != "" || got.GetAboutUpdatedAt() != nil {
		t.Errorf("about = %q (at %v), want both unset", got.GetAbout(), got.GetAboutUpdatedAt())
	}
	if len(repo.writes) != 1 || repo.writes[0].update.About == nil || *repo.writes[0].update.About != "" {
		t.Fatalf("writes = %v, want one empty About meaning unset", repo.writes)
	}
}

func TestHideAboutNeedsAReason(t *testing.T) {
	svc, repo, _ := newService(t)
	repo.players["u1"] = &playerState{id: "u1", about: "текст"}

	_, err := svc.HideAbout(as("mod"), &userv1.HideAboutRequest{UserId: "u1", Reason: "   "})

	if code := domainCode(t, err); code != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", code)
	}
	if len(repo.writes) != 0 {
		t.Errorf("a refused hide reached the repository: %v", repo.writes)
	}
}

func TestHideAboutRemovesTheDescriptionAndTellsThePlayer(t *testing.T) {
	svc, repo, notices := newService(t)
	repo.players["u1"] = &playerState{
		id: "u1", gameName: "Ветер", status: verification.VerificationStatusVerified, about: "оскорбления",
	}

	_, err := svc.HideAbout(as("mod"), &userv1.HideAboutRequest{UserId: "u1", Reason: "  оскорбления  "})
	if err != nil {
		t.Fatalf("HideAbout: %v", err)
	}

	if repo.players["u1"].about != "" || repo.players["u1"].aboutAt != nil {
		t.Errorf("the description survived the hide: %q", repo.players["u1"].about)
	}
	if notices.calls != 1 {
		t.Fatalf("notifications = %d, want 1", notices.calls)
	}
	if notices.title != "Описание профиля скрыто" {
		t.Errorf("title = %q", notices.title)
	}
	want := "Модератор скрыл ваше описание: оскорбления. Вы можете написать новое."
	if notices.message != want {
		t.Errorf("message = %q, want %q", notices.message, want)
	}
}

func TestHideAboutRefusesAnUnknownPlayer(t *testing.T) {
	svc, _, _ := newService(t)

	_, err := svc.HideAbout(as("mod"), &userv1.HideAboutRequest{UserId: "nobody", Reason: "спам"})

	if code := statusCode(t, err); code != codes.NotFound {
		t.Errorf("code = %v, want NotFound", code)
	}
}

func TestHideAboutSurvivesAFailedNotification(t *testing.T) {
	svc, repo, notices := newService(t)
	notices.err = errors.New("the notification service is down")
	repo.players["u1"] = &playerState{id: "u1", status: verification.VerificationStatusVerified, about: "текст"}

	if _, err := svc.HideAbout(as("mod"), &userv1.HideAboutRequest{UserId: "u1", Reason: "спам"}); err != nil {
		t.Fatalf("a failing notification must not fail the hide: %v", err)
	}

	if repo.players["u1"].about != "" {
		t.Errorf("the description survived: %q", repo.players["u1"].about)
	}
}

func TestListPlayersClampsThePage(t *testing.T) {
	cases := []struct {
		name     string
		req      *userv1.ListPlayersRequest
		wantPage int
		wantSize int
	}{
		{"nothing asked", &userv1.ListPlayersRequest{}, 1, 24},
		{"page zero means the first", &userv1.ListPlayersRequest{Page: 0, PageSize: 24}, 1, 24},
		{"a negative page is the first", &userv1.ListPlayersRequest{Page: -3, PageSize: 24}, 1, 24},
		{"the default size", &userv1.ListPlayersRequest{Page: 2}, 2, 24},
		{"at most 48", &userv1.ListPlayersRequest{Page: 1, PageSize: 100}, 1, 48},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, _ := newService(t)

			got, err := svc.ListPlayers(context.Background(), tc.req)
			if err != nil {
				t.Fatalf("ListPlayers: %v", err)
			}

			if repo.gotPage != tc.wantPage || repo.gotSize != tc.wantSize {
				t.Errorf("repository asked for page %d size %d, want %d/%d",
					repo.gotPage, repo.gotSize, tc.wantPage, tc.wantSize)
			}
			if got.GetPage() != int32(tc.wantPage) || got.GetPageSize() != int32(tc.wantSize) {
				t.Errorf("answer says page %d size %d, want %d/%d",
					got.GetPage(), got.GetPageSize(), tc.wantPage, tc.wantSize)
			}
		})
	}
}

func TestListPlayersDropsAQueryTooShortToNarrowAnything(t *testing.T) {
	svc, repo, _ := newService(t)

	if _, err := svc.ListPlayers(context.Background(), &userv1.ListPlayersRequest{Query: "  в  "}); err != nil {
		t.Fatalf("ListPlayers: %v", err)
	}

	if repo.gotFilter.Query != "" {
		t.Errorf("query = %q, want it dropped", repo.gotFilter.Query)
	}
}

func TestListPlayersKeepsAQueryThatNarrowsTheRoster(t *testing.T) {
	svc, repo, _ := newService(t)

	if _, err := svc.ListPlayers(context.Background(), &userv1.ListPlayersRequest{Query: "  вет  "}); err != nil {
		t.Fatalf("ListPlayers: %v", err)
	}

	if repo.gotFilter.Query != "вет" {
		t.Errorf("query = %q, want the trimmed text", repo.gotFilter.Query)
	}
}

func TestListPlayersRejectsAQueryOverTheLimit(t *testing.T) {
	svc, _, _ := newService(t)

	_, err := svc.ListPlayers(context.Background(), &userv1.ListPlayersRequest{
		Query: strings.Repeat("в", maxRosterQueryRunes+1),
	})

	if code := domainCode(t, err); code != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", code)
	}
}

func TestListPlayersMapsTheWindowAndTheOrder(t *testing.T) {
	cases := []struct {
		lastSeen   userv1.LastSeen
		sort       userv1.PlayerSort
		wantWindow model.LastSeenWindow
		wantSort   model.PlayerListSort
	}{
		{userv1.LastSeen_LAST_SEEN_UNSPECIFIED, userv1.PlayerSort_PLAYER_SORT_UNSPECIFIED, model.LastSeenAny, model.PlayerSortLastSeen},
		{userv1.LastSeen_LAST_SEEN_ONLINE, userv1.PlayerSort_PLAYER_SORT_HOURS, model.LastSeenOnline, model.PlayerSortHours},
		{userv1.LastSeen_LAST_SEEN_DAY, userv1.PlayerSort_PLAYER_SORT_NICKNAME, model.LastSeenDay, model.PlayerSortNickname},
		{userv1.LastSeen_LAST_SEEN_WEEK, userv1.PlayerSort_PLAYER_SORT_LAST_SEEN, model.LastSeenWeek, model.PlayerSortLastSeen},
		{userv1.LastSeen_LAST_SEEN_MONTH, userv1.PlayerSort_PLAYER_SORT_UNSPECIFIED, model.LastSeenMonth, model.PlayerSortLastSeen},
		{userv1.LastSeen_LAST_SEEN_INACTIVE, userv1.PlayerSort_PLAYER_SORT_UNSPECIFIED, model.LastSeenInactive, model.PlayerSortLastSeen},
		{userv1.LastSeen(99), userv1.PlayerSort(99), model.LastSeenAny, model.PlayerSortLastSeen},
	}

	for _, tc := range cases {
		svc, repo, _ := newService(t)

		if _, err := svc.ListPlayers(context.Background(), &userv1.ListPlayersRequest{
			LastSeen: tc.lastSeen, Sort: tc.sort,
		}); err != nil {
			t.Fatalf("ListPlayers: %v", err)
		}

		if repo.gotFilter.LastSeen != tc.wantWindow || repo.gotFilter.Sort != tc.wantSort {
			t.Errorf("filter = %v, want window %d sort %d",
				repo.gotFilter, tc.wantWindow, tc.wantSort)
		}
	}
}

func TestListPlayersCarriesTheRowsAndTheCounts(t *testing.T) {
	svc, repo, _ := newService(t)
	repo.page = &model.PlayerListPage{
		Players: []model.PlayerListItem{{
			UserId: "u1", Nickname: "Ветер", IsOnline: true, HoursPlayed: 12.5, About: "лес",
		}},
		TotalSize: 1,
		Page:      1,
		PageSize:  24,
		Counts:    model.LastSeenCounts{All: 1, Online: 1},
	}

	got, err := svc.ListPlayers(context.Background(), &userv1.ListPlayersRequest{})
	if err != nil {
		t.Fatalf("ListPlayers: %v", err)
	}

	if got.GetTotalSize() != 1 || len(got.GetPlayers()) != 1 {
		t.Fatalf("total = %d, players = %d, want 1 and 1", got.GetTotalSize(), len(got.GetPlayers()))
	}

	p := got.GetPlayers()[0]
	if p.GetNickname() != "Ветер" || !p.GetIsOnline() || p.GetHoursPlayed() != 12.5 || p.GetAbout() != "лес" {
		t.Errorf("player = %v", p)
	}
	if got.GetCounts().GetAll() != 1 || got.GetCounts().GetOnline() != 1 {
		t.Errorf("counts = %v", got.GetCounts())
	}
}

func TestListPlayersReportsAFailure(t *testing.T) {
	svc, repo, _ := newService(t)
	repo.listErr = errors.New("mongo is down")

	_, err := svc.ListPlayers(context.Background(), &userv1.ListPlayersRequest{})

	if code := statusCode(t, err); code != codes.Internal {
		t.Errorf("code = %v, want Internal", code)
	}
}

func TestScopeDeclaresTheNewMethods(t *testing.T) {
	svc, _, _ := newService(t)
	scope := svc.Scope()

	if got, ok := scope["/user.v1.UserService/HideAbout"]; !ok || got != "players:moderate" {
		t.Errorf("HideAbout scope = %q (declared %v), want players:moderate", got, ok)
	}
	if got, ok := scope["/user.v1.UserService/UpdateMyAbout"]; !ok || got != interceptor.ScopeAuthenticated {
		t.Errorf("UpdateMyAbout scope = %q (declared %v), want authenticated", got, ok)
	}
	if got, ok := scope["/user.v1.UserService/AdminChangeNickname"]; !ok || got != "players:nickname:write" {
		t.Errorf("AdminChangeNickname scope = %q (declared %v), want players:nickname:write", got, ok)
	}
	if got, ok := scope["/user.v1.UserService/AdminGrantNicknameChange"]; !ok || got != "players:nickname:grant" {
		t.Errorf("AdminGrantNicknameChange scope = %q (declared %v), want players:nickname:grant", got, ok)
	}
}

func TestChangeNicknameRefusesWhileTheCooldownHolds(t *testing.T) {
	svc, repo, _ := newService(t)
	repo.players["u1"] = &playerState{id: "u1", gameName: "Veter", lastChange: time.Now().Add(-time.Hour)}

	_, err := svc.ChangeNickname(as("u1"), &userv1.ChangeNicknameRequest{UserId: "u1", NewNickname: "Dozhd"})

	if code := statusCode(t, err); code != codes.FailedPrecondition {
		t.Errorf("code = %v, want FailedPrecondition", code)
	}
	if len(repo.nickWrites) != 0 {
		t.Errorf("a refused rename reached the repository: %v", repo.nickWrites)
	}
}

func TestAdminChangeNicknameIgnoresTheCooldown(t *testing.T) {
	svc, repo, notices := newService(t)
	recent := time.Now().Add(-24 * time.Hour)
	repo.players["u1"] = &playerState{id: "u1", gameName: "Veter", lastChange: recent}

	got, err := svc.AdminChangeNickname(as("mod"), &userv1.AdminChangeNicknameRequest{
		UserId: "u1", NewNickname: "Dozhd", Reason: "  просьба игрока  ",
	})
	if err != nil {
		t.Fatalf("AdminChangeNickname: %v", err)
	}

	if got.GetOldNickname() != "Veter" || got.GetNewNickname() != "Dozhd" {
		t.Errorf("answer = %q -> %q, want Veter -> Dozhd", got.GetOldNickname(), got.GetNewNickname())
	}
	if got.GetNextChangeAllowedAt() == nil {
		t.Fatal("next_change_allowed_at is unset, want the new cooldown")
	}
	if !got.GetNextChangeAllowedAt().AsTime().After(time.Now().Add(180 * 24 * time.Hour)) {
		t.Errorf("next change at %v, want a fresh 6-month cooldown", got.GetNextChangeAllowedAt().AsTime())
	}

	if len(repo.nickWrites) != 1 {
		t.Fatalf("nickname writes = %d, want 1", len(repo.nickWrites))
	}
	w := repo.nickWrites[0]
	if w.userId != "u1" || w.old != "Veter" || w.new != "Dozhd" {
		t.Errorf("write = %+v, want Veter -> Dozhd for u1", w)
	}
	if !w.lastSeen.After(recent) {
		t.Errorf("recorded change at %v, want the cooldown restarted", w.lastSeen)
	}
	if repo.players["u1"].gameName != "Dozhd" || repo.players["u1"].previous != "Veter" {
		t.Errorf("stored state = %+v, want the old name kept", repo.players["u1"])
	}

	if notices.calls != 1 {
		t.Fatalf("notifications = %d, want 1", notices.calls)
	}
	if notices.title != "Никнейм изменён администратором" {
		t.Errorf("title = %q", notices.title)
	}
	want := "Администратор изменил ваш никнейм с 'Veter' на 'Dozhd'. Причина: просьба игрока"
	if notices.message != want {
		t.Errorf("message = %q, want %q", notices.message, want)
	}
}

func TestAdminChangeNicknameRefusesAnUnknownPlayer(t *testing.T) {
	svc, repo, _ := newService(t)

	_, err := svc.AdminChangeNickname(as("mod"), &userv1.AdminChangeNicknameRequest{
		UserId: "nobody", NewNickname: "Dozhd",
	})

	if code := statusCode(t, err); code != codes.NotFound {
		t.Errorf("code = %v, want NotFound", code)
	}
	if len(repo.nickWrites) != 0 {
		t.Errorf("a refused rename reached the repository: %v", repo.nickWrites)
	}
}

func TestAdminChangeNicknameRefusesANicknameTheGameRejects(t *testing.T) {
	cases := []struct {
		name     string
		nickname string
	}{
		{"empty", ""},
		{"too long", strings.Repeat("a", 16)},
		{"a space", "Dozhd idet"},
		{"punctuation", "Dozhd!"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, _ := newService(t)
			repo.players["u1"] = &playerState{id: "u1", gameName: "Veter"}

			_, err := svc.AdminChangeNickname(as("mod"), &userv1.AdminChangeNicknameRequest{
				UserId: "u1", NewNickname: tc.nickname,
			})

			if code := domainCode(t, err); code != codes.InvalidArgument {
				t.Errorf("code = %v, want InvalidArgument", code)
			}
			if len(repo.nickWrites) != 0 {
				t.Errorf("a refused rename reached the repository: %v", repo.nickWrites)
			}
		})
	}
}

func TestAdminChangeNicknameRefusesTheNameItAlreadyHas(t *testing.T) {
	svc, repo, _ := newService(t)
	repo.players["u1"] = &playerState{id: "u1", gameName: "Veter"}

	_, err := svc.AdminChangeNickname(as("mod"), &userv1.AdminChangeNicknameRequest{
		UserId: "u1", NewNickname: "Veter",
	})

	if code := domainCode(t, err); code != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", code)
	}
	if len(repo.nickWrites) != 0 {
		t.Errorf("a no-op rename reached the repository: %v", repo.nickWrites)
	}
}

func TestAdminChangeNicknameSurvivesAFailedNotification(t *testing.T) {
	svc, repo, notices := newService(t)
	notices.err = errors.New("the notification service is down")
	repo.players["u1"] = &playerState{id: "u1", gameName: "Veter"}

	if _, err := svc.AdminChangeNickname(as("mod"), &userv1.AdminChangeNicknameRequest{
		UserId: "u1", NewNickname: "Dozhd",
	}); err != nil {
		t.Fatalf("a failing notification must not fail the rename: %v", err)
	}

	if repo.players["u1"].gameName != "Dozhd" {
		t.Errorf("nickname = %q, want the rename kept", repo.players["u1"].gameName)
	}
}

func TestAdminGrantNicknameChangeLiftsTheCooldown(t *testing.T) {
	svc, repo, notices := newService(t)
	repo.players["u1"] = &playerState{id: "u1", gameName: "Veter", lastChange: time.Now().Add(-time.Hour)}

	if _, err := svc.AdminGrantNicknameChange(as("mod"), &userv1.AdminGrantNicknameChangeRequest{
		UserId: "u1", Reason: "исключение",
	}); err != nil {
		t.Fatalf("AdminGrantNicknameChange: %v", err)
	}

	if !repo.players["u1"].lastChange.IsZero() {
		t.Errorf("recorded change = %v, want it cleared", repo.players["u1"].lastChange)
	}
	if len(repo.writes) != 1 || !repo.writes[0].update.ClearNicknameCooldown {
		t.Fatalf("writes = %v, want one that clears the cooldown", repo.writes)
	}
	if notices.calls != 1 || notices.title != "Смена никнейма разрешена" {
		t.Errorf("notification = %q (%d calls), want the grant notice", notices.title, notices.calls)
	}

	// The point of the grant: the player's own request now passes the cooldown
	// check that refused it above.
	if _, err := svc.ChangeNickname(as("u1"), &userv1.ChangeNicknameRequest{
		UserId: "u1", NewNickname: "Dozhd",
	}); err != nil {
		t.Fatalf("the player still cannot rename after a grant: %v", err)
	}
	if repo.players["u1"].gameName != "Dozhd" {
		t.Errorf("nickname = %q, want the player's own choice", repo.players["u1"].gameName)
	}
}

func TestAdminGrantNicknameChangeRefusesWhenNothingToGrant(t *testing.T) {
	svc, repo, notices := newService(t)
	repo.players["u1"] = &playerState{id: "u1", gameName: "Veter"}

	_, err := svc.AdminGrantNicknameChange(as("mod"), &userv1.AdminGrantNicknameChangeRequest{
		UserId: "u1",
	})

	if code := domainCode(t, err); code != codes.FailedPrecondition {
		t.Errorf("code = %v, want FailedPrecondition", code)
	}
	if len(repo.writes) != 0 {
		t.Errorf("a refused grant reached the repository: %v", repo.writes)
	}
	if notices.calls != 0 {
		t.Errorf("notifications = %d, want none", notices.calls)
	}
}

func TestAdminGrantNicknameChangeRefusesAnUnknownPlayer(t *testing.T) {
	svc, _, _ := newService(t)

	_, err := svc.AdminGrantNicknameChange(as("mod"), &userv1.AdminGrantNicknameChangeRequest{
		UserId: "nobody",
	})

	if code := statusCode(t, err); code != codes.NotFound {
		t.Errorf("code = %v, want NotFound", code)
	}
}

func TestAdminNicknameReasonIsBounded(t *testing.T) {
	svc, repo, _ := newService(t)
	repo.players["u1"] = &playerState{id: "u1", gameName: "Veter", lastChange: time.Now().Add(-time.Hour)}

	_, err := svc.AdminChangeNickname(as("mod"), &userv1.AdminChangeNicknameRequest{
		UserId: "u1", NewNickname: "Dozhd", Reason: strings.Repeat("я", model.MaxReasonRunes+1),
	})

	if code := domainCode(t, err); code != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", code)
	}
	if len(repo.nickWrites) != 0 {
		t.Errorf("a rename with an oversized reason reached the repository: %v", repo.nickWrites)
	}
}
