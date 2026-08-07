// Package quickact parses GitLab-style quick actions written in task comments
// ("/close", "/assign @msdnna") and describes the built-in command set.
//
// The package is deliberately free of gin and database dependencies: it only
// recognises commands and parses their argument *syntax*. Resolving arguments
// against the workspace (users, columns, tags) and executing them lives in the
// handler layer, so this half stays pure and table-testable.
//
// Custom, workspace-defined commands (workspace_commands) are recognised but
// never executed — see Parse.
package quickact

import "strings"

// ArgKind tells the handler layer how to resolve a command's argument, and the
// frontend how to hint it in the autocomplete popup.
type ArgKind string

// Argument kinds understood by the built-in command set.
const (
	ArgNone      ArgKind = "none"
	ArgUser      ArgKind = "user"      // @login, possibly several
	ArgDate      ArgKind = "date"      // see ParseDate
	ArgText      ArgKind = "text"      // free text to end of line
	ArgTag       ArgKind = "tag"       // tag name(s), comma-separated
	ArgColumn    ArgKind = "column"    // board column name
	ArgMilestone ArgKind = "milestone" // milestone title
	ArgPriority  ArgKind = "priority"  // see ParsePriority
	ArgEstimate  ArgKind = "estimate"  // see ParseEstimate
	ArgTaskRef   ArgKind = "task_ref"  // #123
)

// Command is one built-in quick action.
type Command struct {
	Key         string   `json:"key"`
	Aliases     []string `json:"aliases,omitempty"`
	Arg         ArgKind  `json:"arg"`
	ArgOptional bool     `json:"arg_optional,omitempty"`
	Repeatable  bool     `json:"repeatable,omitempty"`
	Description string   `json:"description"`
	Example     string   `json:"example"`
}

// Registry is the single source of truth for built-in commands: the API hands
// it to clients verbatim, so adding a command here makes it show up in the
// editor popup without any frontend change. Order is the popup order.
var Registry = []Command{
	// Assignees
	{Key: "assign", Aliases: []string{"assignee"}, Arg: ArgUser, Repeatable: true,
		Description: "Назначить исполнителя", Example: "/assign @msdnna"},
	{Key: "unassign", Arg: ArgUser, ArgOptional: true, Repeatable: true,
		Description: "Снять исполнителя (без аргумента — всех)", Example: "/unassign @msdnna"},

	// Dates and estimate
	{Key: "due", Aliases: []string{"due_date"}, Arg: ArgDate,
		Description: "Установить срок", Example: "/due 2026-08-14"},
	{Key: "remove_due", Arg: ArgNone, Description: "Убрать срок", Example: "/remove_due"},
	{Key: "start", Aliases: []string{"start_date"}, Arg: ArgDate,
		Description: "Установить дату начала", Example: "/start завтра"},
	{Key: "remove_start", Arg: ArgNone, Description: "Убрать дату начала", Example: "/remove_start"},
	{Key: "estimate", Arg: ArgEstimate, Description: "Установить оценку", Example: "/estimate 2h30m"},
	{Key: "remove_estimate", Arg: ArgNone, Description: "Убрать оценку", Example: "/remove_estimate"},

	// Plain fields
	{Key: "priority", Arg: ArgPriority, Description: "Установить приоритет", Example: "/priority высокий"},
	{Key: "title", Arg: ArgText, Description: "Изменить заголовок", Example: "/title Новый заголовок"},

	// Tags and milestone
	{Key: "tag", Aliases: []string{"label"}, Arg: ArgTag, Repeatable: true,
		Description: "Добавить тег (через запятую)", Example: "/tag backend, срочно"},
	{Key: "untag", Aliases: []string{"unlabel"}, Arg: ArgTag, Repeatable: true,
		Description: "Убрать тег", Example: "/untag backend"},
	{Key: "milestone", Arg: ArgMilestone, Description: "Назначить майлстоун", Example: "/milestone Релиз 1.0"},
	{Key: "remove_milestone", Arg: ArgNone, Description: "Убрать майлстоун", Example: "/remove_milestone"},

	// Column / lifecycle
	{Key: "move", Aliases: []string{"status"}, Arg: ArgColumn,
		Description: "Перенести в колонку", Example: "/move В процессе"},
	{Key: "close", Aliases: []string{"done"}, Arg: ArgNone,
		Description: "Закрыть задачу", Example: "/close"},
	{Key: "reopen", Aliases: []string{"undone"}, Arg: ArgNone,
		Description: "Переоткрыть задачу", Example: "/reopen"},
	{Key: "archive", Arg: ArgNone, Description: "Архивировать задачу", Example: "/archive"},

	// Relations
	{Key: "relate", Arg: ArgTaskRef, Repeatable: true,
		Description: "Связать с задачей", Example: "/relate #2591"},
	{Key: "blocks", Arg: ArgTaskRef, Repeatable: true,
		Description: "Блокирует задачу", Example: "/blocks #2591"},
	{Key: "blocked_by", Aliases: []string{"blockedby"}, Arg: ArgTaskRef, Repeatable: true,
		Description: "Заблокирована задачей", Example: "/blocked_by #2591"},
	{Key: "duplicates", Arg: ArgTaskRef, Repeatable: true,
		Description: "Дублирует задачу", Example: "/duplicates #2591"},
	{Key: "unlink", Arg: ArgTaskRef, Repeatable: true,
		Description: "Снять связь с задачей", Example: "/unlink #2591"},

	// Hierarchy
	{Key: "subtask", Arg: ArgText, Description: "Создать подзадачу", Example: "/subtask Написать тесты"},
	{Key: "parent", Arg: ArgTaskRef, Description: "Сделать подзадачей задачи", Example: "/parent #2591"},
	{Key: "unparent", Arg: ArgNone, Description: "Открепить от родителя", Example: "/unparent"},
}

// byKey maps every canonical key and alias to its command.
var byKey = func() map[string]*Command {
	m := make(map[string]*Command, len(Registry)*2)
	for i := range Registry {
		cmd := &Registry[i]
		m[cmd.Key] = cmd
		for _, a := range cmd.Aliases {
			m[a] = cmd
		}
	}
	return m
}()

// Lookup resolves a key or alias (case-insensitive, leading slashes trimmed) to
// a built-in command.
func Lookup(key string) (*Command, bool) {
	cmd, ok := byKey[CanonKey(key)]
	return cmd, ok
}

// IsBuiltin reports whether a key collides with a built-in command. The
// workspace dictionary editor rejects such keys: a custom "/close" would look
// like a note to a bot while the backend quietly closed the task.
func IsBuiltin(key string) bool {
	_, ok := byKey[CanonKey(key)]
	return ok
}

// CanonKey normalises a command key to its storage form: no leading slash,
// trimmed, lowercase. Mirror this in the frontend (utils/commands.js).
func CanonKey(raw string) string {
	return strings.ToLower(strings.TrimLeft(strings.TrimSpace(raw), "/"))
}
