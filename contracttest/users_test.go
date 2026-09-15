package contracttest_test

// Конформанс-сьют users (api-schema#9, инкремент 2): все 21 операция контракта
// через generated-клиент против generated-сервера на fake-имплементации.
// Каждая операция проверяет транспорт туда-обратно: путь/параметры/тело
// доходят до имплементации неизменными, код и тело ответа раскладываются
// клиентом в правильный вариант. Плюс транспорные инварианты: strict-binding
// (unknown field → 400) и форма ошибки валидации.

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/zvonilkaRU/api-schema/contracttest"
	server "github.com/zvonilkaRU/api-schema/generated/users/impl/echoserver"
	httpclient "github.com/zvonilkaRU/api-schema/generated/users/impl/httpclient"
	apiclient "github.com/zvonilkaRU/api-schema/generated/users/interfaces/client"
	apiserver "github.com/zvonilkaRU/api-schema/generated/users/interfaces/server"
	// Все подпакеты generated/users/model* объявлены как `package model` —
	// импортируем с явными алиасами (как делает сам generated-клиент).
	model "github.com/zvonilkaRU/api-schema/generated/users/model"
	auth "github.com/zvonilkaRU/api-schema/generated/users/model/auth"
	friends "github.com/zvonilkaRU/api-schema/generated/users/model/friends"
	models "github.com/zvonilkaRU/api-schema/generated/users/model/models"
	profile "github.com/zvonilkaRU/api-schema/generated/users/model/profile"
)

// fakeUsers — имплементация apiserver.Server на замыканиях. Незаглушенный
// метод паникует (nil-интерфейс): тест падает громко, а не проходит молча.
// Компилятор требует все методы контракта — новая операция без сценария
// не соберётся.
type fakeUsers struct {
	apiserver.Server

	registerUser            func(context.Context, *apiclient.RegisterUserRequest) (*apiclient.RegisterUserResponse, error)
	loginUser               func(context.Context, *apiclient.LoginUserRequest) (*apiclient.LoginUserResponse, error)
	refreshToken            func(context.Context, *apiclient.RefreshTokenRequest) (*apiclient.RefreshTokenResponse, error)
	logoutUser              func(context.Context, *apiclient.LogoutUserRequest) (*apiclient.LogoutUserResponse, error)
	confirmEmail            func(context.Context, *apiclient.ConfirmEmailRequest) (*apiclient.ConfirmEmailResponse, error)
	resendEmailConfirmation func(context.Context, *apiclient.ResendEmailConfirmationRequest) (*apiclient.ResendEmailConfirmationResponse, error)
	getCurrentUser          func(context.Context, *apiclient.GetCurrentUserRequest) (*apiclient.GetCurrentUserResponse, error)
	updateCurrentUser       func(context.Context, *apiclient.UpdateCurrentUserRequest) (*apiclient.UpdateCurrentUserResponse, error)
	getUserByID             func(context.Context, *apiclient.GetUserByIDRequest) (*apiclient.GetUserByIDResponse, error)
	listSessions            func(context.Context, *apiclient.ListSessionsRequest) (*apiclient.ListSessionsResponse, error)
	deleteSession           func(context.Context, *apiclient.DeleteSessionRequest) (*apiclient.DeleteSessionResponse, error)
	sendFriendRequest       func(context.Context, *apiclient.SendFriendRequestRequest) (*apiclient.SendFriendRequestResponse, error)
	listIncomingRequests    func(context.Context, *apiclient.ListIncomingRequestsRequest) (*apiclient.ListIncomingRequestsResponse, error)
	acceptFriendRequest     func(context.Context, *apiclient.AcceptFriendRequestRequest) (*apiclient.AcceptFriendRequestResponse, error)
	declineFriendRequest    func(context.Context, *apiclient.DeclineFriendRequestRequest) (*apiclient.DeclineFriendRequestResponse, error)
	listFriends             func(context.Context, *apiclient.ListFriendsRequest) (*apiclient.ListFriendsResponse, error)
	removeFriend            func(context.Context, *apiclient.RemoveFriendRequest) (*apiclient.RemoveFriendResponse, error)
	listOutgoingRequests    func(context.Context, *apiclient.ListOutgoingRequestsRequest) (*apiclient.ListOutgoingRequestsResponse, error)
	cancelFriendRequest     func(context.Context, *apiclient.CancelFriendRequestRequest) (*apiclient.CancelFriendRequestResponse, error)
	getJwks                 func(context.Context, *apiclient.GetJwksRequest) (*apiclient.GetJwksResponse, error)
	healthCheck             func(context.Context, *apiclient.HealthCheckRequest) (*apiclient.HealthCheckResponse, error)
}

func (f *fakeUsers) RegisterUser(ctx context.Context, req *apiclient.RegisterUserRequest) (*apiclient.RegisterUserResponse, error) {
	return f.registerUser(ctx, req)
}

func (f *fakeUsers) LoginUser(ctx context.Context, req *apiclient.LoginUserRequest) (*apiclient.LoginUserResponse, error) {
	return f.loginUser(ctx, req)
}

func (f *fakeUsers) RefreshToken(ctx context.Context, req *apiclient.RefreshTokenRequest) (*apiclient.RefreshTokenResponse, error) {
	return f.refreshToken(ctx, req)
}

func (f *fakeUsers) LogoutUser(ctx context.Context, req *apiclient.LogoutUserRequest) (*apiclient.LogoutUserResponse, error) {
	return f.logoutUser(ctx, req)
}

func (f *fakeUsers) ConfirmEmail(ctx context.Context, req *apiclient.ConfirmEmailRequest) (*apiclient.ConfirmEmailResponse, error) {
	return f.confirmEmail(ctx, req)
}

func (f *fakeUsers) ResendEmailConfirmation(ctx context.Context, req *apiclient.ResendEmailConfirmationRequest) (*apiclient.ResendEmailConfirmationResponse, error) {
	return f.resendEmailConfirmation(ctx, req)
}

func (f *fakeUsers) GetCurrentUser(ctx context.Context, req *apiclient.GetCurrentUserRequest) (*apiclient.GetCurrentUserResponse, error) {
	return f.getCurrentUser(ctx, req)
}

func (f *fakeUsers) UpdateCurrentUser(ctx context.Context, req *apiclient.UpdateCurrentUserRequest) (*apiclient.UpdateCurrentUserResponse, error) {
	return f.updateCurrentUser(ctx, req)
}

func (f *fakeUsers) GetUserByID(ctx context.Context, req *apiclient.GetUserByIDRequest) (*apiclient.GetUserByIDResponse, error) {
	return f.getUserByID(ctx, req)
}

func (f *fakeUsers) ListSessions(ctx context.Context, req *apiclient.ListSessionsRequest) (*apiclient.ListSessionsResponse, error) {
	return f.listSessions(ctx, req)
}

func (f *fakeUsers) DeleteSession(ctx context.Context, req *apiclient.DeleteSessionRequest) (*apiclient.DeleteSessionResponse, error) {
	return f.deleteSession(ctx, req)
}

func (f *fakeUsers) SendFriendRequest(ctx context.Context, req *apiclient.SendFriendRequestRequest) (*apiclient.SendFriendRequestResponse, error) {
	return f.sendFriendRequest(ctx, req)
}

func (f *fakeUsers) ListIncomingRequests(ctx context.Context, req *apiclient.ListIncomingRequestsRequest) (*apiclient.ListIncomingRequestsResponse, error) {
	return f.listIncomingRequests(ctx, req)
}

func (f *fakeUsers) AcceptFriendRequest(ctx context.Context, req *apiclient.AcceptFriendRequestRequest) (*apiclient.AcceptFriendRequestResponse, error) {
	return f.acceptFriendRequest(ctx, req)
}

func (f *fakeUsers) DeclineFriendRequest(ctx context.Context, req *apiclient.DeclineFriendRequestRequest) (*apiclient.DeclineFriendRequestResponse, error) {
	return f.declineFriendRequest(ctx, req)
}

func (f *fakeUsers) ListFriends(ctx context.Context, req *apiclient.ListFriendsRequest) (*apiclient.ListFriendsResponse, error) {
	return f.listFriends(ctx, req)
}

func (f *fakeUsers) RemoveFriend(ctx context.Context, req *apiclient.RemoveFriendRequest) (*apiclient.RemoveFriendResponse, error) {
	return f.removeFriend(ctx, req)
}

func (f *fakeUsers) ListOutgoingRequests(ctx context.Context, req *apiclient.ListOutgoingRequestsRequest) (*apiclient.ListOutgoingRequestsResponse, error) {
	return f.listOutgoingRequests(ctx, req)
}

func (f *fakeUsers) CancelFriendRequest(ctx context.Context, req *apiclient.CancelFriendRequestRequest) (*apiclient.CancelFriendRequestResponse, error) {
	return f.cancelFriendRequest(ctx, req)
}

func (f *fakeUsers) GetJwks(ctx context.Context, req *apiclient.GetJwksRequest) (*apiclient.GetJwksResponse, error) {
	return f.getJwks(ctx, req)
}

func (f *fakeUsers) HealthCheck(ctx context.Context, req *apiclient.HealthCheckRequest) (*apiclient.HealthCheckResponse, error) {
	return f.healthCheck(ctx, req)
}

func TestUsersConformance(t *testing.T) {
	ctx := context.Background()
	fake := &fakeUsers{}
	reg := contracttest.StubRegistry(t, model.ExpectedValidatorNames(), nil)
	srv := contracttest.Serve(t, func(e *echo.Echo) {
		server.NewServerHTTP(fake, reg).Register(e)
	})
	client, err := httpclient.NewClient(srv.URL)
	if err != nil {
		t.Fatalf("client: %v", err)
	}

	// --- helpers -------------------------------------------------------------

	must := func(name string, respCode, want int) {
		if respCode != want {
			t.Fatalf("%s: код ответа = %d, want %d", name, respCode, want)
		}
	}

	alice := models.UserResponse{ID: "01928a44-0000-7000-8000-000000000001", Login: "alice", Email: "alice@example.com", Nickname: "Алиса"}
	ref := models.UserRefResponse{ID: "01928a44-0000-7000-8000-000000000002", Nickname: "Боб", Tag: "AB1CD23"}

	// --- auth ----------------------------------------------------------------

	t.Run("RegisterUser: тело до имплементации, 201 с User обратно", func(t *testing.T) {
		fake.registerUser = func(_ context.Context, req *apiclient.RegisterUserRequest) (*apiclient.RegisterUserResponse, error) {
			if req.Body.Login != "alice" || req.Body.Password != "secret123" || req.Body.Nickname != "Алиса" {
				t.Errorf("сервер получил искажённое тело: %+v", req.Body)
			}

			return &apiclient.RegisterUserResponse{Code: http.StatusCreated, Response201: &auth.RegisterResponseResponse{User: alice}}, nil
		}
		resp, err := client.RegisterUser(ctx, &apiclient.RegisterUserRequest{
			Body: auth.RegisterRequestRequest{Login: "alice", Email: "alice@example.com", Password: "secret123", Nickname: "Алиса"},
		})
		if err != nil {
			t.Fatalf("client: %v", err)
		}
		must("RegisterUser", resp.Code, http.StatusCreated)
		if resp.Response201 == nil || resp.Response201.User.Login != "alice" || resp.Response201.User.ID != alice.ID {
			t.Errorf("клиент не разобрал 201: %+v", resp.Response201)
		}
	})

	t.Run("RegisterUser: 409 (конфликт) раскладывается в Response409", func(t *testing.T) {
		fake.registerUser = func(context.Context, *apiclient.RegisterUserRequest) (*apiclient.RegisterUserResponse, error) {
			return &apiclient.RegisterUserResponse{
				Code:        http.StatusConflict,
				Response409: &model.ErrorResponse{Code: "ALREADY_EXISTS", Message: "login taken"},
			}, nil
		}
		resp, err := client.RegisterUser(ctx, &apiclient.RegisterUserRequest{
			Body: auth.RegisterRequestRequest{Login: "alice", Email: "alice@example.com", Password: "secret123", Nickname: "Алиса"},
		})
		if err != nil {
			t.Fatalf("client: %v", err)
		}
		must("RegisterUser/409", resp.Code, http.StatusConflict)
		if resp.Response409 == nil || resp.Response409.Code != "ALREADY_EXISTS" {
			t.Errorf("клиент не разобрал 409: %+v", resp.Response409)
		}
	})

	t.Run("LoginUser: тело и 200 с токенами", func(t *testing.T) {
		fake.loginUser = func(_ context.Context, req *apiclient.LoginUserRequest) (*apiclient.LoginUserResponse, error) {
			if req.Body.IDentifier != "alice" || req.Body.Password != "secret123" {
				t.Errorf("сервер получил искажённое тело: %+v", req.Body)
			}

			return &apiclient.LoginUserResponse{
				Code: http.StatusOK,
				Response200: &auth.LoginResponseResponse{
					User: alice, AccessToken: "at-1", RefreshToken: "rt-1", ExpiresIn: 900,
				},
			}, nil
		}
		resp, err := client.LoginUser(ctx, &apiclient.LoginUserRequest{
			Body: auth.LoginRequestRequest{IDentifier: "alice", Password: "secret123"},
		})
		if err != nil {
			t.Fatalf("client: %v", err)
		}
		must("LoginUser", resp.Code, http.StatusOK)
		if resp.Response200 == nil || resp.Response200.AccessToken != "at-1" || resp.Response200.RefreshToken != "rt-1" {
			t.Errorf("клиент не разобрал 200: %+v", resp.Response200)
		}
	})

	t.Run("RefreshToken: optional-тело, 200 с новой парой", func(t *testing.T) {
		fake.refreshToken = func(context.Context, *apiclient.RefreshTokenRequest) (*apiclient.RefreshTokenResponse, error) {
			return &apiclient.RefreshTokenResponse{
				Code:        http.StatusOK,
				Response200: &auth.RefreshResponseResponse{AccessToken: "at-2", RefreshToken: "rt-2", ExpiresIn: 900},
			}, nil
		}
		resp, err := client.RefreshToken(ctx, &apiclient.RefreshTokenRequest{})
		if err != nil {
			t.Fatalf("client: %v", err)
		}
		must("RefreshToken", resp.Code, http.StatusOK)
		if resp.Response200 == nil || resp.Response200.RefreshToken != "rt-2" {
			t.Errorf("клиент не разобрал 200: %+v", resp.Response200)
		}
	})

	t.Run("LogoutUser: 204 без тела", func(t *testing.T) {
		fake.logoutUser = func(context.Context, *apiclient.LogoutUserRequest) (*apiclient.LogoutUserResponse, error) {
			return &apiclient.LogoutUserResponse{Code: http.StatusNoContent, Response204: true}, nil
		}
		resp, err := client.LogoutUser(ctx, &apiclient.LogoutUserRequest{})
		if err != nil {
			t.Fatalf("client: %v", err)
		}
		must("LogoutUser", resp.Code, http.StatusNoContent)
	})

	t.Run("ConfirmEmail: токен в теле, 200 с парой токенов", func(t *testing.T) {
		token := strings.Repeat("a", 43)
		fake.confirmEmail = func(_ context.Context, req *apiclient.ConfirmEmailRequest) (*apiclient.ConfirmEmailResponse, error) {
			if req.Body.Token != token {
				t.Errorf("сервер получил искажённый токен: %q", req.Body.Token)
			}

			return &apiclient.ConfirmEmailResponse{
				Code: http.StatusOK,
				Response200: &auth.EmailConfirmResponseResponse{
					User: alice, AccessToken: "at-3", RefreshToken: "rt-3", ExpiresIn: 900,
				},
			}, nil
		}
		resp, err := client.ConfirmEmail(ctx, &apiclient.ConfirmEmailRequest{Body: auth.EmailConfirmRequestRequest{Token: token}})
		if err != nil {
			t.Fatalf("client: %v", err)
		}
		must("ConfirmEmail", resp.Code, http.StatusOK)
		if resp.Response200 == nil || resp.Response200.AccessToken != "at-3" {
			t.Errorf("клиент не разобрал 200: %+v", resp.Response200)
		}
	})

	t.Run("ResendEmailConfirmation: 202 Accepted без тела", func(t *testing.T) {
		fake.resendEmailConfirmation = func(context.Context, *apiclient.ResendEmailConfirmationRequest) (*apiclient.ResendEmailConfirmationResponse, error) {
			return &apiclient.ResendEmailConfirmationResponse{Code: http.StatusAccepted, Response202: true}, nil
		}
		resp, err := client.ResendEmailConfirmation(ctx, &apiclient.ResendEmailConfirmationRequest{
			Body: auth.EmailResendRequestRequest{Email: "alice@example.com"},
		})
		if err != nil {
			t.Fatalf("client: %v", err)
		}
		must("ResendEmailConfirmation", resp.Code, http.StatusAccepted)
	})

	// --- profile / users -------------------------------------------------------

	t.Run("GetCurrentUser: 200 UserResponse", func(t *testing.T) {
		fake.getCurrentUser = func(context.Context, *apiclient.GetCurrentUserRequest) (*apiclient.GetCurrentUserResponse, error) {
			return &apiclient.GetCurrentUserResponse{Code: http.StatusOK, Response200: &alice}, nil
		}
		resp, err := client.GetCurrentUser(ctx, &apiclient.GetCurrentUserRequest{})
		if err != nil {
			t.Fatalf("client: %v", err)
		}
		must("GetCurrentUser", resp.Code, http.StatusOK)
		if resp.Response200 == nil || resp.Response200.Email != "alice@example.com" {
			t.Errorf("клиент не разобрал 200: %+v", resp.Response200)
		}
	})

	t.Run("UpdateCurrentUser: optional-поля тела, 200 обновлённый User", func(t *testing.T) {
		nick := "Алиса_2"
		fake.updateCurrentUser = func(_ context.Context, req *apiclient.UpdateCurrentUserRequest) (*apiclient.UpdateCurrentUserResponse, error) {
			if req.Body.Nickname == nil || *req.Body.Nickname != "Алиса_2" {
				t.Errorf("сервер не получил nickname: %+v", req.Body)
			}
			updated := alice
			updated.Nickname = nick

			return &apiclient.UpdateCurrentUserResponse{Code: http.StatusOK, Response200: &updated}, nil
		}
		resp, err := client.UpdateCurrentUser(ctx, &apiclient.UpdateCurrentUserRequest{
			Body: profile.UpdateUserRequestRequest{Nickname: &nick},
		})
		if err != nil {
			t.Fatalf("client: %v", err)
		}
		must("UpdateCurrentUser", resp.Code, http.StatusOK)
		if resp.Response200 == nil || resp.Response200.Nickname != "Алиса_2" {
			t.Errorf("клиент не разобрал 200: %+v", resp.Response200)
		}
	})

	t.Run("GetUserByID: path-параметр до имплементации, 200 UserRef", func(t *testing.T) {
		fake.getUserByID = func(_ context.Context, req *apiclient.GetUserByIDRequest) (*apiclient.GetUserByIDResponse, error) {
			if req.ID != string(alice.ID) {
				t.Errorf("сервер получил искажённый path-параметр: %q", req.ID)
			}

			return &apiclient.GetUserByIDResponse{Code: http.StatusOK, Response200: &ref}, nil
		}
		resp, err := client.GetUserByID(ctx, &apiclient.GetUserByIDRequest{ID: string(alice.ID)})
		if err != nil {
			t.Fatalf("client: %v", err)
		}
		must("GetUserByID", resp.Code, http.StatusOK)
		if resp.Response200 == nil || resp.Response200.Tag != "AB1CD23" {
			t.Errorf("клиент не разобрал 200: %+v", resp.Response200)
		}
	})

	// --- sessions ---------------------------------------------------------------

	t.Run("ListSessions: query-параметры до имплементации, 200 список+токен", func(t *testing.T) {
		size := int32(7)
		tok := "tok-1"
		next := "tok-2"
		fake.listSessions = func(_ context.Context, req *apiclient.ListSessionsRequest) (*apiclient.ListSessionsResponse, error) {
			if req.PageSize == nil || *req.PageSize != 7 || req.PageToken == nil || *req.PageToken != "tok-1" {
				t.Errorf("сервер получил искажённые query: %+v", req)
			}

			return &apiclient.ListSessionsResponse{
				Code: http.StatusOK,
				Response200: &models.SessionListResponse{
					Items:         []models.SessionResponse{{}},
					NextPageToken: &next,
				},
			}, nil
		}
		resp, err := client.ListSessions(ctx, &apiclient.ListSessionsRequest{PageSize: &size, PageToken: &tok})
		if err != nil {
			t.Fatalf("client: %v", err)
		}
		must("ListSessions", resp.Code, http.StatusOK)
		if resp.Response200 == nil || len(resp.Response200.Items) != 1 || resp.Response200.NextPageToken == nil || *resp.Response200.NextPageToken != "tok-2" {
			t.Errorf("клиент не разобрал 200: %+v", resp.Response200)
		}
	})

	t.Run("DeleteSession: path-параметр, 204", func(t *testing.T) {
		fake.deleteSession = func(_ context.Context, req *apiclient.DeleteSessionRequest) (*apiclient.DeleteSessionResponse, error) {
			if req.ID != "sess-1" {
				t.Errorf("сервер получил искажённый path-параметр: %q", req.ID)
			}

			return &apiclient.DeleteSessionResponse{Code: http.StatusNoContent, Response204: true}, nil
		}
		resp, err := client.DeleteSession(ctx, &apiclient.DeleteSessionRequest{ID: "sess-1"})
		if err != nil {
			t.Fatalf("client: %v", err)
		}
		must("DeleteSession", resp.Code, http.StatusNoContent)
	})

	// --- friends ------------------------------------------------------------------

	t.Run("SendFriendRequest: тег в теле, 201 эхом", func(t *testing.T) {
		fake.sendFriendRequest = func(_ context.Context, req *apiclient.SendFriendRequestRequest) (*apiclient.SendFriendRequestResponse, error) {
			if req.Body.Tag != "AB1CD23" {
				t.Errorf("сервер получил искажённый тег: %q", req.Body.Tag)
			}

			return &apiclient.SendFriendRequestResponse{
				Code:        http.StatusCreated,
				Response201: &friends.SendFriendRequestResponse{Tag: "AB1CD23"},
			}, nil
		}
		resp, err := client.SendFriendRequest(ctx, &apiclient.SendFriendRequestRequest{
			Body: friends.SendFriendRequestRequest{Tag: "AB1CD23"},
		})
		if err != nil {
			t.Fatalf("client: %v", err)
		}
		must("SendFriendRequest", resp.Code, http.StatusCreated)
		if resp.Response201 == nil || resp.Response201.Tag != "AB1CD23" {
			t.Errorf("клиент не разобрал 201: %+v", resp.Response201)
		}
	})

	t.Run("ListFriends: 200 обёртка FriendList", func(t *testing.T) {
		fake.listFriends = func(context.Context, *apiclient.ListFriendsRequest) (*apiclient.ListFriendsResponse, error) {
			return &apiclient.ListFriendsResponse{
				Code: http.StatusOK,
				Response200: &friends.FriendListResponse{
					Items:         []models.UserRefResponse{ref},
					NextPageToken: nil,
				},
			}, nil
		}
		resp, err := client.ListFriends(ctx, &apiclient.ListFriendsRequest{})
		if err != nil {
			t.Fatalf("client: %v", err)
		}
		must("ListFriends", resp.Code, http.StatusOK)
		if resp.Response200 == nil || len(resp.Response200.Items) != 1 || resp.Response200.Items[0].Nickname != "Боб" {
			t.Errorf("клиент не разобрал 200: %+v", resp.Response200)
		}
	})

	t.Run("ListIncomingRequests: 200 обёртка IncomingRequestList", func(t *testing.T) {
		fake.listIncomingRequests = func(context.Context, *apiclient.ListIncomingRequestsRequest) (*apiclient.ListIncomingRequestsResponse, error) {
			return &apiclient.ListIncomingRequestsResponse{
				Code: http.StatusOK,
				Response200: &friends.IncomingRequestListResponse{
					Items:         []models.UserRefResponse{ref},
					NextPageToken: nil,
				},
			}, nil
		}
		resp, err := client.ListIncomingRequests(ctx, &apiclient.ListIncomingRequestsRequest{})
		if err != nil {
			t.Fatalf("client: %v", err)
		}
		must("ListIncomingRequests", resp.Code, http.StatusOK)
		if resp.Response200 == nil || len(resp.Response200.Items) != 1 {
			t.Errorf("клиент не разобрал 200: %+v", resp.Response200)
		}
	})

	t.Run("ListOutgoingRequests: 200 пустая страница", func(t *testing.T) {
		fake.listOutgoingRequests = func(context.Context, *apiclient.ListOutgoingRequestsRequest) (*apiclient.ListOutgoingRequestsResponse, error) {
			return &apiclient.ListOutgoingRequestsResponse{
				Code:        http.StatusOK,
				Response200: &friends.OutgoingRequestListResponse{Items: []models.UserRefResponse{}},
			}, nil
		}
		resp, err := client.ListOutgoingRequests(ctx, &apiclient.ListOutgoingRequestsRequest{})
		if err != nil {
			t.Fatalf("client: %v", err)
		}
		must("ListOutgoingRequests", resp.Code, http.StatusOK)
		if resp.Response200 == nil || len(resp.Response200.Items) != 0 {
			t.Errorf("клиент не разобрал 200: %+v", resp.Response200)
		}
	})

	t.Run("AcceptFriendRequest: path-параметр, 200 UserRef", func(t *testing.T) {
		fake.acceptFriendRequest = func(_ context.Context, req *apiclient.AcceptFriendRequestRequest) (*apiclient.AcceptFriendRequestResponse, error) {
			if req.ID != "fr-1" {
				t.Errorf("сервер получил искажённый path-параметр: %q", req.ID)
			}

			return &apiclient.AcceptFriendRequestResponse{Code: http.StatusOK, Response200: &ref}, nil
		}
		resp, err := client.AcceptFriendRequest(ctx, &apiclient.AcceptFriendRequestRequest{ID: "fr-1"})
		if err != nil {
			t.Fatalf("client: %v", err)
		}
		must("AcceptFriendRequest", resp.Code, http.StatusOK)
		if resp.Response200 == nil || resp.Response200.Tag != "AB1CD23" {
			t.Errorf("клиент не разобрал 200: %+v", resp.Response200)
		}
	})

	t.Run("DeclineFriendRequest: 204", func(t *testing.T) {
		fake.declineFriendRequest = func(context.Context, *apiclient.DeclineFriendRequestRequest) (*apiclient.DeclineFriendRequestResponse, error) {
			return &apiclient.DeclineFriendRequestResponse{Code: http.StatusNoContent, Response204: true}, nil
		}
		resp, err := client.DeclineFriendRequest(ctx, &apiclient.DeclineFriendRequestRequest{ID: "fr-1"})
		if err != nil {
			t.Fatalf("client: %v", err)
		}
		must("DeclineFriendRequest", resp.Code, http.StatusNoContent)
	})

	t.Run("RemoveFriend: 204", func(t *testing.T) {
		fake.removeFriend = func(context.Context, *apiclient.RemoveFriendRequest) (*apiclient.RemoveFriendResponse, error) {
			return &apiclient.RemoveFriendResponse{Code: http.StatusNoContent, Response204: true}, nil
		}
		resp, err := client.RemoveFriend(ctx, &apiclient.RemoveFriendRequest{ID: string(ref.ID)})
		if err != nil {
			t.Fatalf("client: %v", err)
		}
		must("RemoveFriend", resp.Code, http.StatusNoContent)
	})

	t.Run("CancelFriendRequest: 204", func(t *testing.T) {
		fake.cancelFriendRequest = func(context.Context, *apiclient.CancelFriendRequestRequest) (*apiclient.CancelFriendRequestResponse, error) {
			return &apiclient.CancelFriendRequestResponse{Code: http.StatusNoContent, Response204: true}, nil
		}
		resp, err := client.CancelFriendRequest(ctx, &apiclient.CancelFriendRequestRequest{ID: "fr-2"})
		if err != nil {
			t.Fatalf("client: %v", err)
		}
		must("CancelFriendRequest", resp.Code, http.StatusNoContent)
	})

	// --- service ----------------------------------------------------------------

	t.Run("GetJwks: 200 набор ключей", func(t *testing.T) {
		fake.getJwks = func(context.Context, *apiclient.GetJwksRequest) (*apiclient.GetJwksResponse, error) {
			return &apiclient.GetJwksResponse{
				Code: http.StatusOK,
				Response200: &models.JwkSetResponse{
					Keys: []models.JSONWebKeyResponse{{Kty: "EC", Kid: "k1", Use: "sig", Alg: "ES256", Crv: "P-256"}},
				},
			}, nil
		}
		resp, err := client.GetJwks(ctx, &apiclient.GetJwksRequest{})
		if err != nil {
			t.Fatalf("client: %v", err)
		}
		must("GetJwks", resp.Code, http.StatusOK)
		if resp.Response200 == nil || len(resp.Response200.Keys) != 1 || resp.Response200.Keys[0].Kid != "k1" {
			t.Errorf("клиент не разобрал 200: %+v", resp.Response200)
		}
	})

	t.Run("HealthCheck: 200 статус ok", func(t *testing.T) {
		fake.healthCheck = func(context.Context, *apiclient.HealthCheckRequest) (*apiclient.HealthCheckResponse, error) {
			return &apiclient.HealthCheckResponse{
				Code:        http.StatusOK,
				Response200: &model.HealthStatusResponse{Status: "ok"},
			}, nil
		}
		resp, err := client.HealthCheck(ctx, &apiclient.HealthCheckRequest{})
		if err != nil {
			t.Fatalf("client: %v", err)
		}
		must("HealthCheck", resp.Code, http.StatusOK)
		if resp.Response200 == nil || resp.Response200.Status != "ok" {
			t.Errorf("клиент не разобрал 200: %+v", resp.Response200)
		}
	})

	// --- транспорные инварианты (вне конкретной операции) ------------------------

	t.Run("strict binding: unknown field в body → 400 с именем поля", func(t *testing.T) {
		fake.registerUser = func(context.Context, *apiclient.RegisterUserRequest) (*apiclient.RegisterUserResponse, error) {
			t.Error("имплементация не должна вызываться при unknown field")

			return nil, nil
		}
		httpResp, err := http.Post(srv.URL+"/users/v1/auth/register", "application/json",
			strings.NewReader(`{"login":"alice","nope":1}`))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer httpResp.Body.Close() //nolint:errcheck // тест
		if httpResp.StatusCode != http.StatusBadRequest {
			t.Fatalf("код = %d, want 400", httpResp.StatusCode)
		}
		body, _ := io.ReadAll(httpResp.Body)
		// В raw-JSON кавычки экранированы — матчим имя поля и маркер по отдельности.
		if !strings.Contains(string(body), "unknown field") || !strings.Contains(string(body), "nope") {
			t.Errorf("тело не называет поле: %s", body)
		}
	})

	t.Run("inline-валидация: короткий login → 400 с путём до поля", func(t *testing.T) {
		fake.registerUser = func(context.Context, *apiclient.RegisterUserRequest) (*apiclient.RegisterUserResponse, error) {
			t.Error("имплементация не должна вызываться при невалидном теле")

			return nil, nil
		}
		resp, err := client.RegisterUser(ctx, &apiclient.RegisterUserRequest{
			Body: auth.RegisterRequestRequest{Login: "ab", Email: "alice@example.com", Password: "secret123", Nickname: "Алиса"},
		})
		if err != nil {
			t.Fatalf("client: %v", err)
		}
		must("RegisterUser/invalid", resp.Code, http.StatusBadRequest)
		if resp.Response400 == nil || !strings.Contains(resp.Response400.Message, "Login") {
			t.Errorf("ошибка валидации без поля Login: %+v", resp.Response400)
		}
	})
}
