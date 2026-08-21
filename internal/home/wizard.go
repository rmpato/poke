package home

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"

	"github.com/rmpato/poke/internal/ui"
)

// ---------------------------------------------------------------------------
// Wizards
// ---------------------------------------------------------------------------
//
// A wizard is a Huh form run *outside* the Bubble Tea loop, per the stack
// rules: multi-step input is what Huh is for, and nesting it inside a
// screen's Update/View means reimplementing focus, validation and paging by
// hand. RunWizard blocks, returns the collected answers, and leaves the
// caller to decide what to do with them.

// FieldKind selects the input widget for a WizardField.
type FieldKind int

const (
	// FieldText is a single-line text input.
	FieldText FieldKind = iota
	// FieldSelect is a one-of-N picker.
	FieldSelect
	// FieldMultiSelect is an N-of-M picker; Values joins with ", ".
	FieldMultiSelect
	// FieldConfirm is a yes/no toggle; the answer is "true" or "false".
	FieldConfirm
	// FieldNote renders explanatory copy with no input of its own.
	FieldNote
)

// Option is one choice in a select or multi-select field.
type Option struct {
	Label string
	Value string
}

// WizardField is one input. Key is how the answer comes back in Result.
type WizardField struct {
	Key         string
	Title       string
	Description string
	Kind        FieldKind
	Options     []Option
	Placeholder string
	Default     string
	Defaults    []string
	// Required rejects an empty answer rather than accepting a blank value —
	// only meaningful for FieldText.
	Required bool
}

// WizardStep is one page of the wizard. Keep pages short: a page per
// decision reads far better than one page with nine inputs on it.
type WizardStep struct {
	Title       string
	Description string
	Fields      []WizardField
}

// WizardConfig describes one wizard run.
type WizardConfig struct {
	AppName string
	Title   string
	Steps   []WizardStep
}

// Result holds a completed wizard's answers, keyed by WizardField.Key.
type Result struct {
	Values map[string]string
	Multi  map[string][]string
}

// Get returns one answer, or fallback when the key was never filled in.
func (r Result) Get(key, fallback string) string {
	if value, ok := r.Values[key]; ok && value != "" {
		return value
	}
	return fallback
}

// Bool reports whether a FieldConfirm answer was affirmative.
func (r Result) Bool(key string) bool { return r.Values[key] == "true" }

// List returns a multi-select answer.
func (r Result) List(key string) []string { return r.Multi[key] }

// ErrCancelled is returned when the user aborts the wizard.
var ErrCancelled = errors.New("wizard cancelled")

// RunWizard shows cfg as a themed multi-step Huh form and returns the
// answers. A user who quits gets ErrCancelled rather than a half-filled
// Result, so callers can't accidentally act on an abandoned run.
func RunWizard(cfg WizardConfig) (Result, error) {
	result := Result{
		Values: map[string]string{},
		Multi:  map[string][]string{},
	}

	// Huh binds to pointers, so every field needs somewhere stable to land.
	text := map[string]*string{}
	multi := map[string]*[]string{}
	confirm := map[string]*bool{}

	groups := make([]*huh.Group, 0, len(cfg.Steps))
	for index, step := range cfg.Steps {
		fields := make([]huh.Field, 0, len(step.Fields)+1)

		heading := fmt.Sprintf("Step %d of %d — %s", index+1, len(cfg.Steps), step.Title)
		fields = append(fields, huh.NewNote().Title(heading).Description(step.Description))

		for _, field := range step.Fields {
			switch field.Kind {
			case FieldSelect:
				value := new(string)
				*value = field.Default
				text[field.Key] = value
				fields = append(fields, huh.NewSelect[string]().
					Title(field.Title).
					Description(field.Description).
					Options(huhOptions(field.Options)...).
					Value(value))

			case FieldMultiSelect:
				value := new([]string)
				*value = field.Defaults
				multi[field.Key] = value
				fields = append(fields, huh.NewMultiSelect[string]().
					Title(field.Title).
					Description(field.Description).
					Options(huhOptions(field.Options)...).
					Value(value))

			case FieldConfirm:
				value := new(bool)
				*value = field.Default == "true"
				confirm[field.Key] = value
				fields = append(fields, huh.NewConfirm().
					Title(field.Title).
					Description(field.Description).
					Value(value))

			case FieldNote:
				fields = append(fields, huh.NewNote().
					Title(field.Title).
					Description(field.Description))

			default:
				value := new(string)
				*value = field.Default
				text[field.Key] = value
				input := huh.NewInput().
					Title(field.Title).
					Description(field.Description).
					Placeholder(field.Placeholder).
					Value(value)
				if field.Required {
					title := field.Title
					input = input.Validate(func(entered string) error {
						if strings.TrimSpace(entered) == "" {
							return fmt.Errorf("%s is required", strings.ToLower(title))
						}
						return nil
					})
				}
				fields = append(fields, input)
			}
		}
		groups = append(groups, huh.NewGroup(fields...))
	}

	form := huh.NewForm(groups...).WithTheme(ui.HuhTheme()).WithShowHelp(true)
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return Result{}, ErrCancelled
		}
		return Result{}, err
	}

	for key, value := range text {
		result.Values[key] = *value
	}
	for key, value := range confirm {
		result.Values[key] = fmt.Sprintf("%t", *value)
	}
	for key, value := range multi {
		result.Multi[key] = *value
		result.Values[key] = strings.Join(*value, ", ")
	}
	return result, nil
}

func huhOptions(options []Option) []huh.Option[string] {
	out := make([]huh.Option[string], 0, len(options))
	for _, option := range options {
		out = append(out, huh.NewOption(option.Label, ui.Fallback(option.Value, option.Label)))
	}
	return out
}
