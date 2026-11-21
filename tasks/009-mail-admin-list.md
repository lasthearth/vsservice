# 009 — `AdminListMail`: админ видит, что отправил и сколько забрали

Цель: закрыть последний пробел админской части почты. Отправка уже есть —
`POST /v1/mail:compose` (`mail:compose`), получатель `oneof {user_id | broadcast}`,
вложения `oneof {item | coins}`, `expires_at`, `idempotency_key`; кит
разворачивается в item-вложения через `POST /v1/mail:compose-kit`. Не было
только чтения: `ListMail` отдаёт письма вызывающего, поэтому после broadcast
«всем 3000 осколков» проверить доставку можно было только запросом в Mongo.

## Решения

- **Один новый RPC, кода отправки не касаемся.** `AdminListMail`,
  `GET /v1/admin/mail`, scope `mail:read` — чтение отделено от отправки
  (`mail:compose`) и отзыва (`mail:revoke`), как `donate:wallet:read` отделён от
  `donate:coins:add`.
- **Счётчик доставки, а не список получателей.** По каждому письму
  `claimed_count` — число claim-строк в состоянии `claimed`. Уникальный индекс
  `(mail_id, player_id)` делает число строк числом игроков. Считается одной
  агрегацией на страницу; `mail_id` — префикс левого ключа индекса, поэтому
  `$in` идёт по индексу. Ноль означает «ещё никто не забрал», в том числе для
  письма-уведомления без вложений.
- **Вложения инлайном.** `AdminMail.attachments` несёт `game_code`, `quantity`,
  `type`: иначе опечатку в коде предмета не увидеть нигде, кроме как по факту
  потери предмета игроком (игра молча скипает нерезолвимое вложение при claim).
- **Фильтры `recipient` и `broadcast_only`.** `broadcast_only` побеждает
  `recipient`. Диапазон дат не вводим: сортировка по времени и курсор закрывают
  поиск.
- **Пагинация курсорная**, как у `news`: `page_size` 20 по умолчанию, потолок
  100, порядок — `_id desc`, который совпадает с `created_at desc`, потому что
  `mongox` берёт `_id` из времени создания.
- **Сортировка по `_id`, а не по `created_at`** — намеренно: отдельный ключ
  сортировки потребовал бы своего индекса, а `_id` уже упорядочен по времени.
- **Потолков на `quantity`/`amount`/число вложений нет** — решение владельца.
  Защита от опечатки остаётся на форме сайта.

## Отвергнуто

- Справочник `game_code` (`ListItemCodes` из `kits` и позиций магазина) — код
  вводится руками, валидируется игрой при claim (решение владельца).
- Адресация списком / по поселению / по роли на сервере — сайт циклит `compose`
  со своим `idempotency_key` на каждого получателя.
- `AdminGetMail` отдельным RPC — вложения едут в списке.
- Потолки на суммы и число вложений.

## Что сделано

| Слой | Файл |
| --- | --- |
| proto | `proto/mail/v1/mail.proto` — RPC + `AdminListMailRequest/Response`, `AdminMail` |
| модель | `internal/mail/internal/model/admin_list.go` — `AdminMailEntry`, `AdminMailFilter` |
| репозиторий | `internal/mail/internal/repository/mongo/admin_list.go` |
| маппер | `internal/mail/internal/service/mapper.go` (goverter) |
| сервис | `internal/mail/internal/service/admin_list.go` |
| scope | `internal/mail/internal/service/scope.go` — `mail:read` |
| контракт | `docs/v1/openapi.yaml` |

## Вне репозитория

В Logto: permissions `mail:compose` и `mail:read` на API-ресурсе из
`JWT_AUDIENCE`, выдача роли `admin`, и `mail:compose mail:read` в requested
scopes приложения сайта. Без этого вызовы отвечают `PermissionDenied`.

## Проверено

- `scope_test.go`: `VerifyCoverage` падает, если RPC не классифицирован;
  `mail:read` закреплён тестом.
- `admin_list_test.go` (сервис): фильтр, page token и размер страницы доходят до
  репозитория; `claimed_count`, флаг broadcast, вложения и `next_page_token`
  доезжают до proto; `InvalidArgument` для битого токена не превращается в
  `Internal`.
- `admin_list_test.go` (репозиторий): `$match` считает только `state=claimed`
  и только письма страницы; `broadcast_only` побеждает `recipient`.
- Мутационная проверка: обнуление `claimed_count` и удаление строки из
  scope-таблицы роняют соответствующие тесты.

Не проверено этим прогоном: сам запрос к живой Mongo (локальный инстанс не
поднят) и сквозная выдача в игре — проверка на сайте после выдачи permission.