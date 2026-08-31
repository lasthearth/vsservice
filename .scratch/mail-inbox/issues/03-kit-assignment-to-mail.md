---
Type: grilling
Status: resolved
Blocked by: 01
---

# Kit-ассайнмент → письмо (авто), вывод /kit claim

## Question

Как `AssignKitToUser` кладёт письмо с kit-вложением, и как выводится игровой путь `/kit claim` + NATS `kit.granted`/`kit.claimed`.

Решить:
- Точка интеграции в `internal/kit/internal/service/service.go` `AssignKitToUser`: создать письмо вместо/вместе с публикацией `KitGrantedEvent`.
- Что кладётся во вложение — kit-ref (имя кита) целиком, или развёрнутый список предметов кита. Как VintageAPI резолвит содержимое кита при claim (сейчас kit-содержимое живёт в VintageAPI SQLite/DatabaseService).
- Судьба существующего NATS-контракта (стрим `reward-events`, `kit_game_consumer` в VintageAPI): выводится ли, и что заменяет доставку.
- Судьба `KitAssignment` стейт-машины (pending→delivered→claimed) — сливается ли с mail-claim или остаётся параллельно на переходный период.
- Deprecation `/kit claim` в lhgui/VintageAPI.

Зависит от формы вложений (тикет 01) и пересекается с контрактом доставки (тикет 05) — согласовать kit-ref vs развёрнутый список именно здесь.

Резолюция: точка интеграции, форма kit-вложения, судьба NATS-пути и KitAssignment, план вывода `/kit claim`.

## Answer

**Закрыт как OUT OF SCOPE.** Старая система `internal/kit` (`AssignKitToUser`, `/kit claim`, стейт-машина `KitAssignment` pending→delivered→claimed, NATS-стрим `reward-events`/`kit_game_consumer`) признана легаси и в этом эффорте не участвует.

Причины:
- Содержимое кита живёт в VintageAPI SQLite → под моделью «два примитива» (ticket 01, Q1=B) невыразимо без grant-ref, а grant-ref отброшен.
- Способ «выдать кит через письмо» теперь идёт через новый **kit-builder** (Q4=B): захват содержимого сундука в кит с game_code + картинками, хранение в vsservice/Mongo, редактирование на сайте. Донат-кит разворачивается в конкретные `items`-вложения at-compose. Старый ассайнмент-путь не нужен.

Старый `internal/kit` остаётся работать параллельно как deprecated; его демонтаж — отдельная задача вне этой карты.
