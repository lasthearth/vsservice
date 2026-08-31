---
Type: prototype
Status: resolved
Blocked by: 05
---

# UI почты в GuideScreen (lhgui)

## Question

Как выглядит и ведёт себя `MailPage` — новая секция **Mail** в главном окне сервера (`Guide/GuideScreen.cs`), рядом с «Events». Ключевой вопрос — «как это выглядит и ведёт себя», поэтому прототип (call skill "prototype"), артефакт линкуется к тикету.

Решить (через прототип-артефакт, к которому реагируем):
- Список писем: сортировка (непрочитанные/новые сверху), визуальная отметка unread, превью title/sender/дата, индикатор вложений и `expires_at`.
- Детально письмо: body + список вложений с иконками (item/coins/kit), кнопка Claim / Claim-all.
- Пустое/loading состояние (`AtlasUi.EmptyState`).
- Бейдж непрочитанных: сайдбар-навитем `MenuSection.Mail` (мирроринг Events, `badgeCount`) + HUD (`ServerMenuHudGui`, агрегация с наградами — см. Not yet specified).
- Пакеты (`Mail/MailPackets.cs`): RequestMail / MailList / MarkRead / Claim / MailActionResult + enum — зеркало DailyRewardPackets, `[ProtoContract(ImplicitFields=AllPublic)]`.
- ModSystem (`Mail/MailModSystem.cs`): channel `lhgui-mail`, `static Instance`, регистрация типов в `Start`, server-router+`PlayerNowPlaying` push, client-handlers+`RequestState/MarkRead/Claim`, `SetBackend`, `MailModel : ChangeNotifier` с `Unread`.
- Интеграция в `GuideScreen` (enum `MenuSection.Mail`, навитем, `BuildContent`-арм, инстанс-проперти+листенер) и `GuideModSystem.Open()` (`MailModSystem.Instance?.RequestState()`).
- Lang-ключи `lhgui:servermenu-section-mail` + `lhgui:mail-*` в en.json и ru.json (LangParity).

Резолюция: прототип UI + утверждённая раскладка экрана, набор пакетов, точки интеграции в GuideScreen. Финализирует спец.

## Answer

**Раскладка — вариант B (drill-in).** Прототип: `lhgui/docs/mail-page-prototype.html` (throwaway, 3 варианта через `?variant=`; ветка `research/mail-ui-prototype`). Выбран B: список писем на всю ширину плиты → клик уводит в полноэкранный разворот с «‹ Назад». И список, и письмо получают полную ширину читаемой колонки (главный выигрыш над A/C: длинный body и ряд вложений не тесно). Отброшены: A (split — узкий разворТ ~340px, двойной скролл), C (grid+модалка — окно поверх модального `GuideScreen`, дублирование вложений карточка↔модалка).

### `MailPage : StatefulWidget` (файл `Mail/MailPage.cs`, зеркало `DailyRewardsPage`)

Двухрежимный экран, режим в стейте:
- **Список** (по умолчанию): `Column[ Header, Expanded(Scroll(Column(rows))) ]`.
  - `Header`: `Text("lhgui:mail-title", Headline)` + `Text` непрочит-счётчик (`Caption`) + gold h-rule (тот же паттерн, что `DailyRewardsPage.BuildHeader`).
  - Строка письма = `AtlasCard` (accent по стейту, `dimmed` для терминальных), `onTap` → drill. Внутри `Row[ unread-dot | Column[ title, meta-Wrap, attach-Row ] ]`.
    - unread: `unread-dot` (8px `GoldBright` кружок) + title `Label`/`FontWeight.Bold`/`GoldBright`; иначе title `Vellum`, без точки.
    - meta-`Wrap`: `AtlasBadge` статуса + `sender` (`Caption`) + дата (`Caption`) + (broadcast) `AtlasBadge("Всем", Info)` + (expires_at) `Caption` с ⏳ (`SignalRed` если `expired`, иначе `VellumMuted`).
    - attach-`Row` (если есть вложения): coins-чип (`splinter-of-spark.png` + сумма) + по item — `ItemStackDisplay` через `ResolveStack` (тот же паттерн, что `DailyRewardCard.BuildReward`/`ResolveStack`, учёт `type` item/block из 01) + `×qty`.
  - Сортировка: unread/новое сверху. Сортирует **сервер** (снапшот приходит уже упорядоченным) — клиент рисует как есть, как `DailyStatePacket.Cards`.
- **Разворот** (drill): `Column[ back-link, title(Headline), meta-Wrap, h-rule, Expanded(Scroll(body + attach-panel)), Footer(Claim) ]`.
  - back-link «‹ Назад» (`GoldBright`, `onTap` → режим списка).
  - body: `Text`/`Body` (`SoftWrap`, `MaxLines=0`) — plain-текст (Markdown в письмах не требуется — уведомления короткие; если позже понадобится, переиспользовать `GuideScreen.BlockToWidget`).
  - attach-panel: `Container`/`Panel` с рядами вложений (иконка + имя + `×qty`), как модалка C но встроено.
  - Footer/Claim: `unread|read`+есть вложения → кнопка `Забрать` (brass, `onTap` → `System.Claim(mailId)`); `claimed` → `AtlasBadge("Получено", Good)`; `expired`/`revoked` → `AtlasBadge` статуса; чистое уведомление → footer пуст.
  - **MarkRead** — сайд-эффект drill'а: при входе в разворот непрочитанного письма `System.MarkRead(mailId)` (оптимистично гасит unread; сервер подтверждает снапшотом).
- Пустое/loading: `Model.Snapshot==null` (ещё не пришёл) → `AtlasUi.EmptyState("lhgui:mail-loading", "…-loading-hint")`; пришёл, писем 0 → `EmptyState("lhgui:mail-empty", "…-empty-hint")`. (В отличие от DailyRewards, у почты **есть** легитимное пустое состояние — инбокс без писем ≠ «нет секции».)

### Видимость секции (отличие от DailyRewards)

DailyRewards прячет секцию при `Snapshot==null` (в singleplayer события нет — пустое состояние было бы ложью). **Почта показывается всегда, когда бэкенд инжектнут** (`Model` получил хоть один снапшот, включая пустой). Открытый вопрос «показывать ли Mail без бэкенда» решается как у DailyRewards: нет бэкенда → нет снапшота → секция скрыта (`HasMail=false`); есть бэкенд но 0 писем → секция видна, `EmptyState`.

### `Mail/MailPackets.cs` (зеркало `DailyRewardPackets`, `[ProtoContract(ImplicitFields=AllPublic)]`)

- `RequestMailPacket` (client→server, пустой): дай снапшот (на join + на открытие).
- `MailItemPacket`: `Type` (item/block, int/enum из 01), `Code`, `Quantity` — клиент резолвит в стек (как `DailyRewardItem`).
- `MailAttachmentPacket`: `Coins` (int) + `Items` (`List<MailItemPacket>`) — примитивы вложений из 01 (kit-ref нет).
- `MailEntryPacket`: `Id`, `Title`, `Body`, `Sender`, `CreatedAtUnix`, `ExpiresAtUnix` (0=вечное), `Broadcast` (bool), `State` (int→`MailEntryState`), `Attachment` (`MailAttachmentPacket`).
- `MailListPacket` (server→client): `List<MailEntryPacket> Entries` — весь инбокс одним сообщением, уже отсортирован. Отсутствие пакета = нет бэкенда = секция скрыта (как `DailyStatePacket`).
- `MarkReadPacket` (client→server): `MailId`.
- `ClaimMailPacket` (client→server): `MailId`. (`ClaimAll` НЕ вводим — 05: клиент циклит идемпотентный `ClaimMailPacket`; вернуть только если broadcast-тяжёлый инбокс покажет round-trip цену — уже в fog.)
- `MailActionResultPacket` (server→client): `MailId`, `Status` (int→`MailStatus` из 05). Свежий снапшот следует за успехом (как `ClaimResultPacket`).
- enum `MailEntryState { Unread, Read, Claimed, Expired, Revoked }` (per-mail стейт для отрисовки; ≠ `MailStatus` из 05 — тот про исход claim-действия).

### `Mail/MailModSystem.cs` (зеркало `DailyRewardModSystem`)

- `const Channel="lhgui-mail"`; `static Instance`; регистрация типов в `Start`.
- `MailModel : ChangeNotifier`: `Snapshot` (`MailListPacket?`), `HasMail` (`Snapshot!=null`), `Unread` (кол-во `Entries` в `Unread`), `Entries` (readonly список), `LastRefusal` (`MailStatus?`). `Apply(MailListPacket)` / `ApplyRefusal(MailStatus)`. HUD-бейдж и сайдбар читают `Unread`.
- server: router `RequestMailPacket`→`SendState`, `MarkReadPacket`→`backend.MarkReadAsync`, `ClaimMailPacket`→`backend.ClaimAsync`+`MailActionResultPacket`+push снапшота; `PlayerNowPlaying` push (как DailyRewards `OnPlayerNowPlaying`, но БЕЗ `NotePlayerVisit` — у почты нет attendance); `RunBackend` fire-and-forget off-tick, `Send` через `EnqueueMainThreadTask`.
- `SetBackend(IMailBackend)` — сиам из 05 (3 метода: `GetMailAsync`→`MailListPacket?`, `MarkReadAsync`, `ClaimAsync`); lhgui НЕ несёт реализацию.
- client: handlers `MailListPacket`→`Model.Apply`, `MailActionResultPacket`→refusal если `Status!=Granted`; `RequestState()`, `MarkRead(id)`, `Claim(id)`.

### Интеграция в `GuideScreen`

- `enum MenuSection { Events, Mail }` (+`Mail`).
- `GuideRootState`: `Mail => MailModSystem.Instance`, `HasMail => Mail?.Model.HasMail == true`; листенер `Mail.Model.AddListener(OnMailChanged)` в `InitState`/снять в `Dispose` (как `Events`/`OnEventsChanged`).
- Навитем в `BuildSidebar` после Events, перед категориями: `GuideNavItem("lhgui:servermenu-section-mail", "textures/icons/envelope.svg", selected==Mail, ()=>Select(Mail), badgeCount: Mail?.Model.Unread ?? 0)`. (Нужна svg-иконка конверта — новый ассет.)
- `BuildContent`: `MenuSection.Mail when Mail != null => new MailPage(Capi, Mail)`.
- Дефолтный выбор в `InitState`: приоритет не меняем (Events→первый гайд); Mail не форсим первым — почта не «живое событие с таймером».
- `GuideModSystem.Open()`: добавить `Mail.MailModSystem.Instance?.RequestState();` рядом с `DailyRewardModSystem.Instance?.RequestState()` (resync перед показом).

### HUD-бейдж (`ServerMenuHudGui`)

Сейчас `Badge => DailyRewardModSystem.Instance?.Model.Available ?? 0`. Агрегировать: `Badge => (DailyRewards.Available) + (Mail.Unread)`. Слушать оба `Model` (add/remove listener на оба). **Агрегация — из fog «Not yet specified»**: подтверждена как простая сумма (непрочит. почта + незабранные награды), один брасс-бейдж. Тонкость (раздельные бейджи/цвета) не нужна — YAGNI.

### Lang-ключи (en.json + ru.json, LangParity)

`lhgui:servermenu-section-mail` («Mail»/«Почта»), `lhgui:mail-title`, `lhgui:mail-unread` (счётчик, `{0}`), `lhgui:mail-empty` + `lhgui:mail-empty-hint`, `lhgui:mail-loading` + `lhgui:mail-loading-hint`, `lhgui:mail-back` («‹ Back»/«‹ Назад»), `lhgui:mail-claim` («Claim»/«Забрать»), `lhgui:mail-broadcast` («All»/«Всем»), `lhgui:mail-notification` («Notification»/«Уведомление»), статусы `lhgui:mail-state-unread|read|claimed|expired|revoked`, `lhgui:mail-expires` (`{0}` — форматированный остаток), refuse-ключи по `MailStatus` (`lhgui:mail-refuse-alreadyclaimed|expired|revoked|unavailable`).

### Новые ассеты

- `textures/icons/envelope.svg` — иконка секции Mail (тинт-совместимый SVG-глиф, как `scroll.svg`/`circle-dot.svg`).
- coins переиспользуют существующий `textures/splinter-of-spark.png`.

**Спец готов к хендоффу.** Реализация — вне карты (при реализации: skill `add-feature`; DailyRewards — прямой референс всех паттернов).
