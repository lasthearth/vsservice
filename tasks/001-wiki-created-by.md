# 001 — Wiki: постоянный автор статьи

Цель: у статьи сейчас одно поле `author_name`, и каждая правка его перезаписывает. Нужен автор, который задаётся при создании и не меняется. `author_name` остаётся «последним редактором».

Ветка: agent/001 от feat/wiki-backend.

Файлы (менять только эти + сгенерированные):
- proto/wiki/v1/wiki.proto
- internal/wiki/internal/model/article.go
- internal/wiki/internal/dto/article.go
- internal/wiki/internal/service/suggestion.go
- internal/wiki/internal/service/service_test.go
- internal/pkg/migrate/migrations.go
- сгенерированное: gen/…, internal/wiki/internal/repository/repomapper/mapper.go, internal/wiki/internal/service/sermapper/mapper.go, openapi (если Makefile его генерирует)

## Решения
1. proto, message Article, новое поле:
   ```proto
   // Subject of the user who created the article. Set once, never changes.
   string created_by = 17;
   ```
   Комментарий у `author_name = 15` оставить: «Display name of the last editor.» → «Subject of the last editor.»
2. model.Article: поле `CreatedBy string` рядом с `AuthorName`.
   - `NewArticle`: `article.CreatedBy = authorName`. Заодно удалить лишнюю строку `_ = now`.
   - `ApplyEdit`: `edit.CreatedBy = a.CreatedBy` (ввод клиента не может поменять автора).
   - `Restore`: `restored.CreatedBy = a.CreatedBy`.
   - Новый метод (модель меняется только своими методами):
     ```go
     // AssignAuthor records who wrote a page that did not exist before. The
     // author also counts as the last editor until someone else edits it.
     func (a *Article) AssignAuthor(author string) {
     	a.CreatedBy = author
     	a.AuthorName = author
     }
     ```
3. dto.Article: `CreatedBy string `bson:"created_by,omitempty"`` после `AuthorName`. Revision не трогать.
4. service/suggestion.go, `applySuggestion`:
   - новая статья: перед `proposal.Publish()` вызвать `proposal.AssignAuthor(suggestion.AuthorName)` — автор статьи = игрок, который её предложил;
   - правка: в `article.ApplyEdit(proposal, …)` вместо `reviewer` передать `suggestion.AuthorName` — последний редактор = игрок, чей текст применён. Параметр `reviewer`, если станет неиспользуемым, убрать из сигнатуры и вызова.
   - `fromProto` в service.go НЕ читает `created_by`: клиент не задаёт автора.
5. Миграция: в `migrations` добавить запись `Version: 3`, `Description: "wiki: backfill created_by from the oldest revision"`, `Down` — no-op как у остальных. Логика `Up` (коллекция `wiki_articles`), замороженные локальные типы, как в существующих миграциях:
   ```go
   type oldWikiRevision struct { AuthorName string `bson:"author_name"` }
   type oldWikiArticle struct {
   	ID         any               `bson:"_id"`
   	AuthorName string            `bson:"author_name"`
   	Revisions  []oldWikiRevision `bson:"revisions"`
   }
   ```
   Для документов с фильтром `{"created_by": {"$exists": false}}`: история хранится от новых к старым, поэтому автор = `AuthorName` последнего элемента `Revisions`, если он не пустой; иначе `AuthorName` документа. Пусто — документ пропустить. `UpdateOne` по `_id` с `$set: {created_by: …}`. Повторный запуск ничего не меняет.
6. Генерация: `make generate` (buf + goverter). Мапперы goverter подхватят поле по имени — руками сгенерированное не править.

## Тесты (service_test.go, в стиле существующих)
- создание статьи: `GetCreatedBy()` = id вызывающего;
- правка другим пользователем: `created_by` прежний, `author_name` = новый редактор;
- восстановление версии: `created_by` прежний;
- одобрение предложения новой статьи: `created_by` и `author_name` = автор предложения, не ревьюер;
- одобрение правки: `created_by` прежний, `author_name` = автор предложения.

## Проверка исполнителем
`make generate && make lint && make test && make build` — всё зелёное.

## Готово, когда
- ответ `GET /v1/wiki/articles/{id}` содержит `created_by`;
- тесты выше есть и проходят;
- миграция 3 добавлена в конец списка, старые записи не тронуты.

## Не делать
- не менять Revision, права доступа, другие RPC;
- не переименовывать `author_name`;
- никаких «улучшений по пути».

## Отчёт
≤10 строк: что сделано, что нет, вывод `make lint/test/build`. Обязательно: `git commit` + `git push -u origin agent/001`.
