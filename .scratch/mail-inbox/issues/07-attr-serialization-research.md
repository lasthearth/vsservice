---
Type: research
Status: resolved
Blocked by:
---

# Формат сериализации `ItemStack.Attributes` → base64 `TreeAttribute` (capturekit ↔ ResolveStack)

## Question

Кит-захват (мод в игре) должен снять содержимое сундука/инвентаря в переносимый снимок, а VintageAPI at-claim обязан воспроизвести стек **байт-в-байт** через `ResolveStack`-паттерн, зафиксированный в тикете 05: `attr_snapshot` пусто ⇒ чистый стек; непусто ⇒ `TreeAttribute.FromBytes(base64decode)` + `MergeTree`, снятие transition-state, `TryGiveItemstack`→`SpawnItemEntity`.

Найти **факты** (не решения) о формате, чтобы контракт `kits` (тикет 09) мог его зафиксировать:

- Как VS сериализует `ItemStack.Attributes` (`ITreeAttribute`) в байты: `ToBytes(BinaryWriter)` / `TreeAttribute.ToBytes()`? Что именно попадает в поток (полный tree с типами узлов)?
- Что должно лежать в снимке помимо attr-байтов: `Collectible.Code` (namespace!), `EnumItemClass` (item vs block — тикет 01 поправка: `ItemAttachment.type`), `StackSize`. Достаточно ли этой тройки + attr для полного восстановления?
- Транзиентные поля, которые НЕ должны ехать (`transitionstate` — 05 уже снимает at-claim; что ещё: temperature, freshness-таймеры?).
- Существующий прецедент: мод `Kit` (`RiderProjects/Kit`) команда `savekit` снимает `Code/ItemClass/Quantity` БЕЗ attr — что теряется на зачарованных/контейнерных предметах, и как именно дополнить сериализацией attr.
- Кросс-язык: C#-писатель кладёт base64-строку в Mongo; Go-читатель (vsservice) её только пробрасывает во вложение (не парсит). VintageAPI-читатель (at-claim) декодит. Подтвердить, что Go никогда не интерпретирует байты — только транспорт строки.

Резолюция фиксирует: точный C#-вызов сериализации/десериализации, набор полей снимка одного предмета, список транзиентных полей на выброс, подтверждение что vsservice-Go — только транспорт base64. Это разблокирует контракт 09.

## Answer

Факты (источники: `anegostudios/vsapi` master; прецеденты `RiderProjects/Kit`, `RiderProjects/VintageAPI`).

- **Сериализация:** `stack.Attributes.ToBytes(BinaryWriter)` (`TreeAttribute.cs` L240) → base64. Пишет для каждого узла `attrId(byte)+key(string)+value.ToBytes`, затем терминатор `0` (`TerminateWrite` L1117). Полное самоописывающее дерево с типами узлов (id 1–16: Int/Long/Double/Float/String/Tree/Itemstack/ByteArray/Bool/…массивы). Вложенные trees (6) и itemstacks (7, напр. `contents` контейнера) рекурсивно входят в поток → зачарования, прочность, содержимое контейнеров, температура, transition-таймеры круглятся.
- **Десериализация:** `stack.Attributes.FromBytes(BinaryReader)` (L186, overwrite — так делает Kit) читает до `0`. `MergeTree(ITreeAttribute)` (L993) — для *частичного* оверлея (так делает DailyRewards с JSON). Для байтового снимка точный вызов — `FromBytes` overwrite.
- **Поля снимка одного предмета:** `Code` (с namespace, `stack.Collectible.Code.ToString()`) + `ItemClass` (item/block, `Collectible.ItemClass`) + `StackSize` + attr-base64 (`Attributes.ToBytes()`, пусто при `Count==0`). Этого достаточно для полного восстановления: `new ItemStack(GetItem/GetBlock(code), size)` + применение attr-байтов. **`ItemStack.Id` не переносить** — это runtime-id, невалиден между серверами; именно поэтому нужен строковый Code (Kit так и делает).
- **Прецедент Kit теряет:** `SerializedItem` (kit.cs L29) хранит `ItemCode/StackSize/AttributesBase64` — **нет ItemClass**, namespace угадывается `GetItem ?? GetBlock` (ломается на code, существующем и как item, и как block). Правка ticket-01 (`ItemAttachment.type`) совпадает с DailyRewards (`ResolveStack` ветвит по `Type`, не угадывает). Дополнить = добавить `ToBytes`→base64 при захвате (Kit `savekit` в игре его не снимает вообще; VintageAPI `OnKitCreate` L496-503 уже снимает).
- **Транзиентные (не должны ехать / стрип перед выдачей):** `transitionstate` (subtree `createdTotalHours/lastUpdatedTotalHours/freshHours[]/transitionHours[]/transitionedHours[]`, абсолютные календарные часы — иначе еда испорчена при выдаче) — стрипается обоими прецедентами через `HasAttribute`+`RemoveAttribute("transitionstate")`, включая `contents["0"]` контейнера. `temperature` (subtree `temperature/temperatureLastUpdate/cooldownSpeed`, тоже calendar-relative) — **прецеденты НЕ стрипают**; решение по нему за тикетом 09. `ItemStack.TempAttributes` вообще не сериализуется (`ToBytes` его не пишет).
- **Go — чистый транспорт:** подтверждено. `internal/mail/` в vsservice ещё не существует; base64 — обычная строка вложения, Go её не парсит. Формат — .NET `BinaryReader` поток (length-prefixed UTF, little-endian), не Go-парсибельный без реимплементации `BinaryReader`. vsservice читает из источника, кладёт в Mongo, отдаёт C#-читателю (VintageAPI) дословно.
- **VintageAPI-метод (цитировать):** base64-путь — `NatsKitsSystem.GiveKitItemsToPlayer` (`Systems/Kit/KitModSystem.cs` L360: `FromBase64String`→`BinaryReader`→`Attributes.FromBytes` L382, `RemoveTransitionState` L384-392, `TryGiveItemstack`→`SpawnItemEntity` L400-403). Паттерн-путь — `MongoDailyRewardBackend.ResolveStack` (`DailyRewardsManager/MongoDailyRewardBackend.cs` L429). Замечание: Kit-захват использует `stream.GetBuffer()` (лишние хвостовые нули; безвредно т.к. `FromBytes` стоп на `0`); чистая форма — `ToBytes()` (`ToArray()`).

Контекст: подробности и построчные ссылки — `.scratch/mail-inbox/research/07-attr-serialization-findings.md`. Ветка: `research/attr-serialization`.
