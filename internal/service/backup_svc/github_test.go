package backup_svc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func withTestServer(handler http.Handler, fn func()) {
	srv := httptest.NewServer(handler)
	defer srv.Close()

	origBase := githubBaseURL
	origAPI := githubAPIBaseURL
	githubBaseURL = srv.URL
	githubAPIBaseURL = srv.URL
	defer func() {
		githubBaseURL = origBase
		githubAPIBaseURL = origAPI
	}()

	fn()
}

func TestPollOnce(t *testing.T) {
	Convey("pollOnce", t, func() {
		Convey("授权成功返回 token", func() {
			withTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck,gosec // test HTTP handler
					"access_token": "ghu_test_token_123",
					"token_type":   "bearer",
				})
			}), func() {
				token, slowDown, done, err := pollOnce("test-device-code")
				So(err, ShouldBeNil)
				So(done, ShouldBeTrue)
				So(slowDown, ShouldBeFalse)
				So(token, ShouldEqual, "ghu_test_token_123")
			})
		})

		Convey("authorization_pending 继续轮询", func() {
			withTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck,gosec // test HTTP handler
					"error":             "authorization_pending",
					"error_description": "The authorization request is still pending.",
				})
			}), func() {
				token, slowDown, done, err := pollOnce("test-device-code")
				So(err, ShouldBeNil)
				So(done, ShouldBeFalse)
				So(slowDown, ShouldBeFalse)
				So(token, ShouldBeEmpty)
			})
		})

		Convey("slow_down 标记减速", func() {
			withTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck,gosec // test HTTP handler
					"error": "slow_down",
				})
			}), func() {
				token, slowDown, done, err := pollOnce("test-device-code")
				So(err, ShouldBeNil)
				So(done, ShouldBeFalse)
				So(slowDown, ShouldBeTrue)
				So(token, ShouldBeEmpty)
			})
		})

		Convey("expired_token 返回错误", func() {
			withTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck,gosec // test HTTP handler
					"error": "expired_token",
				})
			}), func() {
				_, _, _, err := pollOnce("test-device-code")
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "授权码已过期")
			})
		})

		Convey("access_denied 返回错误", func() {
			withTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck,gosec // test HTTP handler
					"error": "access_denied",
				})
			}), func() {
				_, _, _, err := pollOnce("test-device-code")
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "用户拒绝了授权")
			})
		})

		Convey("未知错误返回 error_description", func() {
			withTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck,gosec // test HTTP handler
					"error":             "incorrect_client_credentials",
					"error_description": "The client_id is incorrect.",
				})
			}), func() {
				_, _, _, err := pollOnce("test-device-code")
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "The client_id is incorrect.")
			})
		})

		Convey("非 200 状态码返回错误", func() {
			withTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("server error")) //nolint:errcheck,gosec // test HTTP handler
			}), func() {
				_, _, _, err := pollOnce("test-device-code")
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "HTTP 500")
			})
		})

		Convey("无效 JSON 返回解析错误", func() {
			withTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte("not valid json")) //nolint:errcheck,gosec // test HTTP handler
			}), func() {
				_, _, _, err := pollOnce("test-device-code")
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "解析响应失败")
			})
		})

		Convey("空 token 返回错误", func() {
			withTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck,gosec // test HTTP handler
					"access_token": "",
				})
			}), func() {
				_, _, _, err := pollOnce("test-device-code")
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "GitHub 返回空 token")
			})
		})

		Convey("请求发送正确参数", func() {
			var capturedMethod, capturedAccept, capturedContentType string
			var capturedClientID, capturedDeviceCode, capturedGrantType string
			withTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedMethod = r.Method
				capturedAccept = r.Header.Get("Accept")
				capturedContentType = r.Header.Get("Content-Type")
				r.ParseForm() //nolint:errcheck,gosec // test HTTP handler
				capturedClientID = r.PostForm.Get("client_id")
				capturedDeviceCode = r.PostForm.Get("device_code")
				capturedGrantType = r.PostForm.Get("grant_type")

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck,gosec // test HTTP handler
					"error": "authorization_pending",
				})
			}), func() {
				_, _, _, err := pollOnce("my-device-code")
				So(err, ShouldBeNil)
				So(capturedMethod, ShouldEqual, "POST")
				So(capturedAccept, ShouldEqual, "application/json")
				So(capturedContentType, ShouldEqual, "application/x-www-form-urlencoded")
				So(capturedClientID, ShouldEqual, githubClientID)
				So(capturedDeviceCode, ShouldEqual, "my-device-code")
				So(capturedGrantType, ShouldEqual, "urn:ietf:params:oauth:grant-type:device_code")
			})
		})
	})
}

func TestPollDeviceAuth(t *testing.T) {
	Convey("PollDeviceAuth", t, func() {
		Convey("轮询后成功获取 token", func() {
			var callCount atomic.Int32
			withTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				n := callCount.Add(1)
				if n < 3 {
					json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck,gosec // test HTTP handler
						"error": "authorization_pending",
					})
				} else {
					json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck,gosec // test HTTP handler
						"access_token": "ghu_success",
					})
				}
			}), func() {
				ctx := context.Background()
				token, err := PollDeviceAuth(ctx, "test-code", 1)
				So(err, ShouldBeNil)
				So(token, ShouldEqual, "ghu_success")
				So(callCount.Load(), ShouldEqual, 3)
			})
		})

		Convey("context 取消时停止轮询", func() {
			withTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck,gosec // test HTTP handler
					"error": "authorization_pending",
				})
			}), func() {
				ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
				defer cancel()
				_, err := PollDeviceAuth(ctx, "test-code", 1)
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "授权已取消")
			})
		})

		Convey("错误立即返回", func() {
			withTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck,gosec // test HTTP handler
					"error": "expired_token",
				})
			}), func() {
				ctx := context.Background()
				_, err := PollDeviceAuth(ctx, "test-code", 1)
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "授权码已过期")
			})
		})

		Convey("slow_down 增加轮询间隔", func() {
			var callTimes []time.Time
			var callCount atomic.Int32
			withTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				callTimes = append(callTimes, time.Now())
				w.Header().Set("Content-Type", "application/json")
				n := callCount.Add(1)
				if n == 1 {
					json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck,gosec // test HTTP handler
						"error": "slow_down",
					})
				} else {
					json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck,gosec // test HTTP handler
						"access_token": "ghu_after_slowdown",
					})
				}
			}), func() {
				ctx := context.Background()
				token, err := PollDeviceAuth(ctx, "test-code", 1)
				So(err, ShouldBeNil)
				So(token, ShouldEqual, "ghu_after_slowdown")
				So(len(callTimes), ShouldEqual, 2)
				gap := callTimes[1].Sub(callTimes[0])
				So(gap, ShouldBeGreaterThanOrEqualTo, 5*time.Second)
			})
		})
	})
}

func TestStartDeviceFlow(t *testing.T) {
	Convey("StartDeviceFlow", t, func() {
		Convey("成功发起 Device Flow", func() {
			var capturedPath, capturedMethod string
			withTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedPath = r.URL.Path
				capturedMethod = r.Method
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck,gosec // test HTTP handler
					"device_code":      "dc_abc123",
					"user_code":        "ABCD-1234",
					"verification_uri": "https://github.com/login/device",
					"expires_in":       900,
					"interval":         5,
				})
			}), func() {
				info, err := StartDeviceFlow()
				So(err, ShouldBeNil)
				So(capturedPath, ShouldEqual, "/login/device/code")
				So(capturedMethod, ShouldEqual, "POST")
				So(info.DeviceCode, ShouldEqual, "dc_abc123")
				So(info.UserCode, ShouldEqual, "ABCD-1234")
				So(info.VerificationURI, ShouldEqual, "https://github.com/login/device")
				So(info.ExpiresIn, ShouldEqual, 900)
				So(info.Interval, ShouldEqual, 5)
			})
		})

		Convey("interval 最小为 5 秒", func() {
			withTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck,gosec // test HTTP handler
					"device_code":      "dc_abc123",
					"user_code":        "ABCD-1234",
					"verification_uri": "https://github.com/login/device",
					"expires_in":       900,
					"interval":         2,
				})
			}), func() {
				info, err := StartDeviceFlow()
				So(err, ShouldBeNil)
				So(info.Interval, ShouldEqual, 5)
			})
		})

		Convey("GitHub 返回错误", func() {
			withTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck,gosec // test HTTP handler
					"error":             "unauthorized_client",
					"error_description": "The client is not authorized.",
				})
			}), func() {
				_, err := StartDeviceFlow()
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "The client is not authorized.")
			})
		})
	})
}

func TestGetGitHubUser(t *testing.T) {
	Convey("GetGitHubUser", t, func() {
		Convey("成功获取用户信息", func() {
			var capturedPath, capturedAuth string
			withTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedPath = r.URL.Path
				capturedAuth = r.Header.Get("Authorization")
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck,gosec // test HTTP handler
					"login":      "testuser",
					"avatar_url": "https://avatars.githubusercontent.com/u/123",
				})
			}), func() {
				user, err := GetGitHubUser("test-token")
				So(err, ShouldBeNil)
				So(capturedPath, ShouldEqual, "/user")
				So(capturedAuth, ShouldEqual, "Bearer test-token")
				So(user.Login, ShouldEqual, "testuser")
				So(user.AvatarURL, ShouldEqual, "https://avatars.githubusercontent.com/u/123")
			})
		})

		Convey("非 200 返回错误", func() {
			withTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			}), func() {
				_, err := GetGitHubUser("bad-token")
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "401")
			})
		})
	})
}

func TestListBackupGists(t *testing.T) {
	Convey("ListBackupGists", t, func() {
		Convey("过滤出含备份文件的 Gist", func() {
			var capturedPath, capturedAuth string
			withTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedPath = r.URL.Path
				capturedAuth = r.Header.Get("Authorization")
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode([]map[string]any{ //nolint:errcheck,gosec // test HTTP handler
					{
						"id":          "gist-1",
						"description": "OpsKat Backup",
						"updated_at":  "2025-01-01T00:00:00Z",
						"html_url":    "https://gist.github.com/gist-1",
						"files": map[string]any{
							gistBackupFilename: map[string]string{"filename": gistBackupFilename},
						},
					},
					{
						"id":          "gist-2",
						"description": "Other gist",
						"updated_at":  "2025-01-02T00:00:00Z",
						"html_url":    "https://gist.github.com/gist-2",
						"files": map[string]any{
							"other.md": map[string]string{"filename": "other.md"},
						},
					},
				})
			}), func() {
				gists, err := ListBackupGists("test-token")
				So(err, ShouldBeNil)
				So(capturedPath, ShouldEqual, "/gists")
				So(capturedAuth, ShouldEqual, "Bearer test-token")
				So(len(gists), ShouldEqual, 1)
				So(gists[0].ID, ShouldEqual, "gist-1")
				So(gists[0].Description, ShouldEqual, "OpsKat Backup")
			})
		})

		Convey("无备份 Gist 返回空列表", func() {
			withTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode([]map[string]any{ //nolint:errcheck,gosec // test HTTP handler
					{
						"id":    "gist-1",
						"files": map[string]any{"readme.md": map[string]string{"filename": "readme.md"}},
					},
				})
			}), func() {
				gists, err := ListBackupGists("test-token")
				So(err, ShouldBeNil)
				So(gists, ShouldBeEmpty)
			})
		})
	})
}

func TestGetGistContent(t *testing.T) {
	Convey("GetGistContent", t, func() {
		Convey("成功读取备份内容", func() {
			var capturedPath string
			withTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedPath = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck,gosec // test HTTP handler
					"files": map[string]any{
						gistBackupFilename: map[string]string{
							"content": `{"version":1}`,
						},
					},
				})
			}), func() {
				content, err := GetGistContent("test-token", "test-gist-id")
				So(err, ShouldBeNil)
				So(capturedPath, ShouldEqual, "/gists/test-gist-id")
				So(string(content), ShouldEqual, `{"version":1}`)
			})
		})

		Convey("无备份文件返回错误", func() {
			withTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck,gosec // test HTTP handler
					"files": map[string]any{
						"other.txt": map[string]string{"content": "hello"},
					},
				})
			}), func() {
				_, err := GetGistContent("test-token", "test-gist-id")
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "未找到备份文件")
			})
		})

		Convey("无 token 也能请求", func() {
			var capturedAuth string
			withTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedAuth = r.Header.Get("Authorization")
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck,gosec // test HTTP handler
					"files": map[string]any{
						gistBackupFilename: map[string]string{"content": "data"},
					},
				})
			}), func() {
				content, err := GetGistContent("", "test-gist-id")
				So(err, ShouldBeNil)
				So(capturedAuth, ShouldBeEmpty)
				So(string(content), ShouldEqual, "data")
			})
		})
	})
}

func TestCreateOrUpdateGist(t *testing.T) {
	Convey("CreateOrUpdateGist", t, func() {
		Convey("创建新 Gist", func() {
			var capturedPath, capturedMethod, capturedAuth string
			withTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedPath = r.URL.Path
				capturedMethod = r.Method
				capturedAuth = r.Header.Get("Authorization")
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck,gosec // test HTTP handler
					"id":          "new-gist-id",
					"description": "OpsKat Backup",
					"updated_at":  "2025-01-01T00:00:00Z",
					"html_url":    "https://gist.github.com/new-gist-id",
				})
			}), func() {
				info, err := CreateOrUpdateGist("test-token", "", []byte("backup data"))
				So(err, ShouldBeNil)
				So(capturedPath, ShouldEqual, "/gists")
				So(capturedMethod, ShouldEqual, "POST")
				So(capturedAuth, ShouldEqual, "Bearer test-token")
				So(info.ID, ShouldEqual, "new-gist-id")
			})
		})

		Convey("更新已有 Gist", func() {
			var capturedPath, capturedMethod string
			withTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedPath = r.URL.Path
				capturedMethod = r.Method
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck,gosec // test HTTP handler
					"id":          "existing-id",
					"description": "Updated",
					"updated_at":  "2025-01-02T00:00:00Z",
					"html_url":    "https://gist.github.com/existing-id",
				})
			}), func() {
				info, err := CreateOrUpdateGist("test-token", "existing-id", []byte("new data"))
				So(err, ShouldBeNil)
				So(capturedPath, ShouldEqual, "/gists/existing-id")
				So(capturedMethod, ShouldEqual, "PATCH")
				So(info.ID, ShouldEqual, "existing-id")
			})
		})

		Convey("API 错误返回详细信息", func() {
			withTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"message":"Forbidden"}`)) //nolint:errcheck,gosec // test HTTP handler
			}), func() {
				_, err := CreateOrUpdateGist("test-token", "", []byte("data"))
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "403")
			})
		})
	})
}
