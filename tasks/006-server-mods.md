# 006 — Моды сервера: список в админке, данные с ModDB, архив «Скачать всё»

Цель: страница сайта `/mods` показывает моды игрового сервера, а модераторы ведут список во вкладке админки без выкладки сайта. По ссылке на ModDB бэкенд сам подтягивает название, автора, логотип, версии и ссылки на файлы. Игрок качает каждый мод по кнопке или все обязательные одним архивом.

Фронт уже сделан под этот контракт (landing, ветка `feat/players-and-seo`: `src/app/entities/server-mod`, `src/app/features/mods`). Макеты — артефакт «Моды сервера — Last Hearth». **Имена полей, путей и значений enum ниже менять нельзя** — иначе фронт не сойдётся.

Ветка: agent/006 от master.

Перед началом: AGENTS.md и skill `add-feature` (RPC, goverter, fx, публичные методы, проверка через make).

## Что уже есть и что учесть

- Права: запись — тем же, кто публикует новости и события, scope `news:create` (как `internal/event/internal/service/scope.go`). Новых scope в Logto не заводим.
- Публичные методы — в `publicMethods` интерцептора (как `GetEvent`, `ListEvents`).
- Свой HTTP-маршрут без gRPC — через `mux.HandlePath`, как `/logtohook` в `internal/server/server.go`.
- ModDB API: `GET https://mods.vintagestory.at/api/mod/{assetid | urlalias | modidstr}` → `{"statuscode":"200","mod":{...}}`; при отсутствии мода `statuscode` = `"404"`. Нужные поля `mod`: `name`, `author`, `text` (HTML-описание), `logofile`, `urlalias`, `assetid`, `side` (`both` / `client` / `server`), `releases[]`: `modidstr`, `modversion`, `tags` (версии игры, например `1.21.5`), `mainfile` (прямая ссылка на файл), `filename`, `created`. CORS у ModDB нет — поэтому ходит бэкенд, а не браузер.
- Размер файла ModDB не отдаёт: брать `Content-Length` из `HEAD mainfile` (при добавлении, смене версии и `RefreshMods`). Не удалось — `file_size = 0`, это не ошибка.

## proto (`proto/mod/v1/mod.proto`, `package mod.v1`)

```proto
// ModService is the list of game-server mods shown on the site's /mods page.
// Reading the public list and the archive is open; managing it reuses the news
// scope, because the people who publish news run the site's content.
service ModService {
  // Public: mods players see, in display order. HIDDEN mods are left out.
  rpc ListMods(ListModsRequest) returns (ListModsResponse) {
    option (google.api.http) = {get: "/v1/mods"};
  }

  // Staff: every mod including HIDDEN, each with its recent releases.
  // Errors: UNAUTHENTICATED (401), PERMISSION_DENIED (403): no news:create
  rpc ListModsAdmin(ListModsAdminRequest) returns (ListModsAdminResponse) {
    option (google.api.http) = {get: "/v1/mods/manage"};
  }

  // Staff: look a mod up on ModDB before adding it. Nothing is stored.
  //
  // query accepts a ModDB link (https://mods.vintagestory.at/carryon,
  // https://mods.vintagestory.at/show/mod/4405), an alias, a mod id or an asset id.
  //
  // Errors:
  //   - INVALID_ARGUMENT (400): empty query or a link to another site
  //   - NOT_FOUND (404): ModDB has no such mod
  //   - UNAVAILABLE (503): ModDB did not answer in 10 s
  rpc LookupMod(LookupModRequest) returns (LookupModResponse) {
    option (google.api.http) = {get: "/v1/mods/lookup"};
  }

  // Staff: add a mod at a chosen version. It goes to the top of the list.
  //
  // Errors:
  //   - INVALID_ARGUMENT (400): unknown version, UNSPECIFIED audience, note over 160 chars
  //   - NOT_FOUND (404): ModDB has no such mod
  //   - ALREADY_EXISTS (409): the mod is already in the list
  //   - UNAVAILABLE (503): ModDB did not answer
  rpc AddMod(AddModRequest) returns (Mod) {
    option (google.api.http) = {post: "/v1/mods" body: "*"};
  }

  // Staff: change version, audience or note. Unset fields stay as they are.
  //
  // Errors: INVALID_ARGUMENT (400), NOT_FOUND (404), UNAVAILABLE (503) when a new
  // version has to be fetched from ModDB
  rpc UpdateMod(UpdateModRequest) returns (Mod) {
    option (google.api.http) = {patch: "/v1/mods/{id}" body: "*"};
  }

  // Staff: remove a mod from the list. Errors: NOT_FOUND (404)
  rpc DeleteMod(DeleteModRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = {delete: "/v1/mods/{id}"};
  }

  // Staff: set the display order. ids must hold every mod exactly once.
  // Errors: INVALID_ARGUMENT (400) when ids differ from the stored set
  rpc ReorderMods(ReorderModsRequest) returns (ListModsAdminResponse) {
    option (google.api.http) = {put: "/v1/mods/order" body: "*"};
  }

  // Staff: re-read every mod from ModDB: name, author, logo, side, summary,
  // releases and latest_version. The chosen versions are never changed.
  // A mod ModDB fails on keeps its old data; the call still succeeds.
  rpc RefreshMods(RefreshModsRequest) returns (ListModsAdminResponse) {
    option (google.api.http) = {post: "/v1/mods:refresh" body: "*"};
  }
}

// Who needs the mod.
enum ModAudience {
  MOD_AUDIENCE_UNSPECIFIED = 0;
  // Without it the game will not join the server.
  MOD_AUDIENCE_REQUIRED = 1;
  // Client-side extra players may install.
  MOD_AUDIENCE_OPTIONAL = 2;
  // Server-only; not shown to players and not in the archive.
  MOD_AUDIENCE_HIDDEN = 3;
}

// Where the mod runs, as ModDB states it.
enum ModSide {
  MOD_SIDE_UNSPECIFIED = 0;
  MOD_SIDE_BOTH = 1;
  MOD_SIDE_CLIENT = 2;
  MOD_SIDE_SERVER = 3;
}

message ModRelease {
  string version = 1;                         // "2.0.0"
  repeated string game_versions = 2;          // ModDB tags: "1.21.4", "1.21.5"
  string file_url = 3;                        // ModDB mainfile
  string file_name = 4;                       // "CarryOn-1.21_v2.0.0.zip"
  int64 file_size = 5;                        // bytes; 0 when unknown
  google.protobuf.Timestamp released_at = 6;
}

message Mod {
  string id = 1;                              // releases[].modidstr, e.g. "carryon"
  string name = 2;
  string author = 3;
  string logo_url = 4;                        // empty when the mod has no logo
  string moddb_url = 5;                       // https://mods.vintagestory.at/<urlalias>, else /show/mod/<assetid>
  ModSide side = 6;
  ModAudience audience = 7;
  string note = 8;                            // staff's Russian one-liner, ≤160 chars
  string summary = 9;                         // first sentence of ModDB text, plain, ≤200 chars
  ModRelease release = 10;                    // the version the server runs
  string latest_version = 11;                 // newest ModDB version; empty when release is the newest
  int32 position = 12;                        // 0 = first
  google.protobuf.Timestamp updated_at = 13;
  repeated ModRelease releases = 14;          // newest first, ≤15; only in ListModsAdmin, ReorderMods, RefreshMods
}

// The "download everything" archive: REQUIRED mods only.
message ModArchive {
  string url = 1;                             // "/v1/mods/archive.zip"
  int32 count = 2;                            // required mods in it
  int64 size = 3;                             // sum of their file_size; 0 when any is unknown
  google.protobuf.Timestamp updated_at = 4;   // newest updated_at among them
}

message ListModsRequest {}
message ListModsResponse {
  repeated Mod mods = 1;
  ModArchive archive = 2;                     // unset when there is no required mod
}
message ListModsAdminRequest {}
message ListModsAdminResponse { repeated Mod mods = 1; }
message LookupModRequest { string query = 1; }
message LookupModResponse {
  Mod mod = 1;                                // audience UNSPECIFIED, release unset
  repeated ModRelease releases = 2;           // newest first, ≤15
}
message AddModRequest {
  string query = 1;                           // same forms as LookupMod
  string version = 2;                         // one of the releases
  ModAudience audience = 3;
  string note = 4;
}
message UpdateModRequest {
  string id = 1;
  optional string version = 2;
  optional ModAudience audience = 3;
  optional string note = 4;
}
message DeleteModRequest { string id = 1; }
message ReorderModsRequest { repeated string ids = 1; }
message RefreshModsRequest {}
```

JSON идёт как во всём API: имена полей snake_case, enum — полными именами (`MOD_AUDIENCE_REQUIRED`), `int64` — строкой.

## Домен `internal/mod`

- Коллекция `server_mods`, документ на мод: все поля `Mod` плюс снимок `releases` (≤15 последних). Индекс уникальный по `id`.
- Клиент ModDB — в `internal/pkg/moddb` (таймаут 10 с, свой `User-Agent: lasthearth-vsservice`): `GetMod(ctx, idOrAlias)`, разбор ссылки в id, превращение HTML `text` в одну фразу (снять теги, раскрыть сущности, обрезать по первой точке или 200 символов).
- `AddMod` ставит мод первым, сдвигая остальные (`position + 1`) одним `UpdateMany`.
- `ListMods` — только `REQUIRED` и `OPTIONAL`, по `position`; `releases` в ответе пусто. `archive` считается по `REQUIRED`.
- `latest_version` — `releases[0].version`, если он не равен выбранной версии, иначе пусто.
- Проверка `note`: обрезать пробелы, ≤160 символов, без управляющих символов.

## Архив `GET /v1/mods/archive.zip` (публичный, `mux.HandlePath`)

- ZIP (`archive/zip`, метод Store — моды уже zip) с файлами `release.file_url` всех `REQUIRED` модов в порядке списка; имя файла в архиве — `release.file_name`. Распаковывать моды внутри не нужно: игрок кладёт эти файлы в папку `Mods` как есть.
- Ключ кеша — хеш списка пар `(id, version)` обязательных модов. Собранный архив хранить во временной папке сервиса; пока ключ тот же, отдавать готовый файл (`http.ServeContent`, поддержка `Range`), при смене ключа собирать заново, старый удалять. Одновременные запросы ждут одну сборку (`singleflight`).
- Не скачался хоть один файл — `502` и ничего не кешировать (неполный архив хуже ошибки).
- Заголовки: `Content-Type: application/zip`, `Content-Disposition: attachment; filename="lasthearth-mods.zip"`, `Cache-Control: public, max-age=300`.
- Нет обязательных модов — `404`.

## Проверка

1. `make proto`, `make generate`, `make lint`, `make test`.
2. Тесты: разбор ссылок ModDB (4 формы и чужой сайт), фраза из HTML, `latest_version`, порядок после `AddMod` / `ReorderMods` (неполный и лишний id → 400), ключ кеша архива меняется от версии и не меняется от `note`.
3. Руками: `LookupMod?query=https://mods.vintagestory.at/carryon` → мод и версии; `AddMod` → он первый в `/v1/mods`; `archive.zip` открывается и содержит zip-файлы модов.
