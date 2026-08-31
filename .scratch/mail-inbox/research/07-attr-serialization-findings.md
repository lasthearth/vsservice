# Findings — ItemStack.Attributes ↔ base64 TreeAttribute serialization

Ticket: `07-attr-serialization-research.md` (effort mail-inbox)
Branch: `research/attr-serialization`
Facts only — no design decisions.

Primary sources: `anegostudios/vsapi` (game API, master). Precedent code: `RiderProjects/Kit`,
`RiderProjects/VintageAPI`. All line refs verified this session.

---

## 1. Exact C# serialize / deserialize calls

### The attributes tree (`ITreeAttribute` on `ItemStack.Attributes`)

Source: `vsapi/Datastructures/AttributeTree/TreeAttribute.cs`

- **Write:** `TreeAttribute.ToBytes(BinaryWriter stream)` — L240. Iterates entries, for each writes
  `byte attrId` + `string key` + the attribute value's own `ToBytes`, then a terminating `0` byte
  (`TerminateWrite`, L1117 → `writer.Write((byte)0)`).
- Convenience `byte[] ToBytes()` — L214: wraps a `MemoryStream`+`BinaryWriter` and returns `ms.ToArray()`.
- **Read:** `TreeAttribute.FromBytes(BinaryReader stream)` — L186. `attributes.Clear()`, then loops
  `while ((attrId = ReadByte()) != 0)` reading `key = ReadString()`, resolves the attribute type from
  `AttributeIdMapping[attrId]`, `Activator.CreateInstance`, `attr.FromBytes(stream)`. Stops at the `0`
  terminator. Depth guard at 30.
- `FromBytes(byte[] data)` — L228: wraps a `MemoryStream`+`BinaryReader`.
- Static `CreateFromBytes(byte[])` — L164: builds a fresh `TreeAttribute` from bytes.

**What goes into the stream:** the full attribute tree with self-describing node types. Attribute type IDs
(registered in the static ctor, L131-153):
`1 Int, 2 Long, 3 Double, 4 Float, 5 String, 6 Tree, 7 Itemstack, 8 ByteArray, 9 Bool, 10 StringArray,
11 IntArray, 12 FloatArray, 13 DoubleArray, 14 TreeArray, 15 LongArray, 16 BoolArray`.
Nested trees (id 6) and nested itemstacks (id 7, e.g. container `contents`) recurse. So enchant data,
container contents, durability, temperature, transition timers — every synced attribute — round-trips.

### Restore into a live stack (ResolveStack pattern)

```csharp
var data = Convert.FromBase64String(base64);
using var ms = new MemoryStream(data);
using var reader = new BinaryReader(ms);
stack.Attributes.FromBytes(reader);   // overwrites the fresh stack's attr tree
// then strip transients (see §3), then TryGiveItemstack -> SpawnItemEntity
```

The design in ticket 05 says "MergeTree". Two valid restore idioms exist:
- **`FromBytes` overwrite** — what Kit uses (VintageAPI `GiveKitItemsToPlayer` L382). Replaces the whole
  attr tree of the new stack. Correct when the base64 is a full snapshot.
- **`MergeTree(ITreeAttribute)`** — `TreeAttribute.cs` L993. Merges source into an existing tree
  key-by-key (`MergeAttribute` L1015: missing key → `Clone()`; type-mismatch → throws; nested tree →
  recurse). DailyRewards uses this (`MongoDailyRewardBackend.MergeAttributes` L463) because its source
  is a *partial* JSON overlay, not a full byte snapshot — it does
  `new JsonObject(item.Attributes).ToAttribute()` then `stack.Attributes.MergeTree(tree)`.

For a byte snapshot, `FromBytes` overwrite is the exact call; `MergeTree` is the JSON-overlay path.

### ItemStack whole-stack byte format (for reference — NOT what the snapshot should use)

`vsapi/Common/Collectible/ItemStack.cs`:
- `ItemStack.ToBytes(BinaryWriter)` — L329: writes `(int)Class`, `Id`, `stacksize`, then
  `stackAttributes.ToBytes(stream)`.
- `ItemStack.FromBytes(BinaryReader)` — L341: reads `Class`, `Id`, `stacksize`, `stackAttributes.FromBytes`.

**`Id` is a runtime numeric block/item id, assigned per-world at load — NOT portable across servers or
mod-list changes.** That is why the snapshot must carry the **string Code**, not `ItemStack.ToBytes`.
Reconstruct via `world.GetItem(code)` / `world.GetBlock(code)` → `new ItemStack(collectible, size)`, then
apply attr bytes. This is exactly what Kit does.

---

## 2. Per-item snapshot field set

For one item, enough to fully reconstruct:

| Field | Type | Source of truth | Why |
|---|---|---|---|
| **Code** (with namespace) | string | `stack.Collectible.Code.ToString()` → `"game:pickaxe-copper"` | portable identity; feeds `AssetLocation` → GetItem/GetBlock |
| **ItemClass** (item vs block) | `EnumItemClass` (item/block) | `stack.Collectible.ItemClass` | the two namespaces can hold the same code; must not be guessed |
| **StackSize** | int | `stack.StackSize` | quantity |
| **attr snapshot** | base64(TreeAttribute.ToBytes) or empty | `stack.Attributes.ToBytes()` | enchants, durability, container contents, names, etc. |

**Is Code + Class + Size + attr enough?** Yes. `new ItemStack(collectible, size)` reconstructs a plain
stack; `stack.Attributes.FromBytes(...)` layers back every synced attribute. Nested item stacks inside
containers are carried by attr id 7 inside the tree, so containers restore too. Nothing else per-item is
needed for a full reconstruction of a synced/saved stack.

**Precedent gap — Kit's `SerializedItem` (`Kit/Systems/Kit/kit.cs` L29):** stores only
`ItemCode`, `StackSize`, `AttributesBase64` — **no ItemClass**. At claim it disambiguates by
`GetItem(code) ?? GetBlock(code)` (`GiveKitItemsToPlayer` L364-366). This *guesses* the namespace and is
wrong for any code that exists as both an item and a block. DailyRewards fixed this by carrying an
explicit `Type` field (`DailyRewardItem.Type`, `ResolveStack` L434 branches on `BlockType`). The ticket-01
amendment (`ItemAttachment.type` item/block) matches the DailyRewards approach, not Kit's guess.

---

## 3. Transient fields that must NOT travel / must be stripped

Not saved at all (never in the byte stream): `ItemStack.TempAttributes` (`ItemStack.cs` L83, a separate
`tempAttributes` tree, "not synchronized, not saved"). It is never serialized by `ToBytes`, so it can't leak.

Fields that ARE in `Attributes` (so they DO serialize) and are time-relative — must be removed before
giving, or the item arrives pre-aged:

- **`transitionstate`** — the perish/ripen/dry/cure/ferment timer tree. Written lazily by
  `Collectible.UpdateAndGetTransitionStatesNative` (`Collectible.cs` L2968-2996): a subtree with
  `createdTotalHours`, `lastUpdatedTotalHours`, `freshHours[]`, `transitionHours[]`, `transitionedHours[]`.
  These are absolute game-calendar hours; stale snapshot ⇒ food spoils on delivery. Both precedents strip
  it: `RemoveTransitionState` deletes the whole `"transitionstate"` key
  (Kit L407-419; DailyRewards L481-485).
- **Container inner `transitionstate`** — a container stores its cargo under `"contents"` (a tree of
  itemstack attrs, key `"0"`, `"1"`, ...). Each inner stack has its own `transitionstate`. Both precedents
  reach one level in and strip it: `stack.Attributes.GetTreeAttribute("contents")?.GetItemstack("0")
  ?.Attributes` → `RemoveTransitionState` (Kit L386-392; DailyRewards L454-458). Note both only touch
  slot `"0"` — deeper/other slots are not swept.
- **`temperature`** — a subtree `{ temperature: float, temperatureLastUpdate: double, cooldownSpeed:
    float }` (`Collectible.GetTemperature` L3260-3284: reads `temperatureLastUpdate`, cools relative to
  `world.Calendar.TotalHours`, rewrites it). Also calendar-relative. **Neither precedent strips it** — they
  only strip `transitionstate`. Facts: if the temperature subtree travels, the item keeps its captured
  temperature and cools from capture time on first `GetTemperature`; a hot ingot could arrive hot. Whether
  to strip it is a design call (ticket 09), not established precedent.

Strip mechanism (both precedents): `ITreeAttribute.HasAttribute("transitionstate")` guard then
`.RemoveAttribute("transitionstate")`.

---

## 4. Cross-language: is Go pure transport?

Yes — and today there is no Go code touching attributes at all. Confirmed:
- `internal/mail/` **does not exist yet** in vsservice (no mail domain on disk this session). The base64
  string is a plain string field on the future attachment; nothing in the mail path parses or interprets it.
- Precedent for the same shape: DailyRewards stores item snapshots (`DailyClaimItemDoc`, `Code/Type/Quantity`)
  as opaque documents in Mongo; the C# side is the only reader. Go/vsservice writes the mail row and never
  decodes attachment bytes.
- The byte format is a C# `BinaryReader`/`BinaryWriter` stream (length-prefixed .NET UTF strings via
  `ReadString`/`Write(string)`, little-endian primitives). It is **not** a Go-parseable format without
  reimplementing .NET's `BinaryReader`. Go must treat it as an opaque base64 string: read from source, store
  in Mongo, hand back to the C# reader (VintageAPI) verbatim. Any Go-side parse would be a mistake.

Contract fact for ticket 09: vsservice-Go is pure transport of the base64 string end to end.

---

## 5. VintageAPI method that already does ResolveStack + attr restore

Two concrete precedents to cite:

- **`MongoDailyRewardBackend.ResolveStack(DailyRewardItem item, int dayIndex)`** —
  `VintageAPI/DailyRewardsManager/MongoDailyRewardBackend.cs` **L429**. Type-branch item/block
  (never guessed, L434), `new ItemStack(collectible, qty)`, `MergeAttributes` (L463, JSON-overlay via
  `MergeTree`), `RemoveTransitionState` on stack + container slot 0 (L453-458), returns the stack; caller
  `GiveItemsAsync` L412 does `TryGiveItemstack(stack, true)` else `SpawnItemEntity`. This is the closest
  match to the ticket-05 pattern except the attr source is JSON, not base64 bytes.

- **`NatsKitsSystem.GiveKitItemsToPlayer(IServerPlayer, KitDefinition)`** —
  `VintageAPI/Systems/Kit/KitModSystem.cs` **L360**. This is the base64-bytes path:
  `GetItem ?? GetBlock` (L364), `new ItemStack(collectible, StackSize)` (L374),
  `Convert.FromBase64String` → `BinaryReader` → `stack.Attributes.FromBytes(reader)` (L379-382),
  `RemoveTransitionState` on stack + container slot 0 (L384-392), `TryGiveItemstack(stack, true)` else
  `SpawnItemEntity` (L400-403). **Cite this one for the base64 byte format specifically.**
  Capture side: `OnKitCreate` L496-503 / `OnKitEditAdd` L278-284:
  `if (stack.Attributes is { Count: > 0 })` → `ToBytes(writer)` → `Convert.ToBase64String(buf)`.

**Capture-side gotcha in Kit:** both capture sites use `stream.GetBuffer()` (Kit L283, L501), not
`ms.ToArray()`. `GetBuffer()` returns the full backing array including unused capacity → the base64 can carry
trailing zero bytes. Harmless on read because `FromBytes` stops at the first `0` terminator, but the stored
string is longer than necessary. `TreeAttribute.ToBytes()` (the parameterless overload) uses `ToArray()` and
is the clean form.

---

## Summary table for ticket 09 contract

| Concern | Fact |
|---|---|
| Serialize | `stack.Attributes.ToBytes(BinaryWriter)` → base64; empty when `Attributes.Count == 0` |
| Deserialize | `stack.Attributes.FromBytes(BinaryReader)` (overwrite) — or `MergeTree` for a partial overlay |
| Snapshot fields | Code (namespaced string) + ItemClass (item/block) + StackSize + attr-base64 |
| Reconstruct | `new ItemStack(GetItem/GetBlock(code), size)` then apply attr bytes — never `ItemStack.Id` |
| Strip | `transitionstate` (stack + container `contents["0"]`); temperature NOT stripped by precedent |
| Go role | pure transport of opaque base64 string; never parses .NET byte stream |
| Cite | VintageAPI `NatsKitsSystem.GiveKitItemsToPlayer` (base64 path), `MongoDailyRewardBackend.ResolveStack` (pattern) |
