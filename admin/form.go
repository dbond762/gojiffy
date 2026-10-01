package admin

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/dbond762/gojiffy"
	"github.com/dbond762/gojiffy/view"
)

// Parse reads the request field by field: trims the value, stores it in the
// record, makes the declared checks and only then calls the field's own Parse.
// The value is stored even when a check fails — the form comes back with what
// was typed, not with what was there before — except a value outside Choices:
// that one was not typed but forged, and the record keeps what it had. A field
// that fails a check does not get to Parse: one error per field is enough, and
// the first is the one to fix. A readonly field is skipped whole — no value,
// no checks, no Parse: whatever came for it was forged, the input only shows.
//
// store answers the LookupChoices fields and may be nil when there are none; the
// error is the store's own, or a field with LookupChoices and no store to ask.
func (rs Resource[T]) Parse(r *http.Request, store gojiffy.Chooser, item T, creating bool) (T, map[string]string, error) {
	errs := map[string]string{}
	for _, f := range rs.Fields {
		if !f.inForm() || f.Readonly != nil && f.Readonly(item) {
			continue
		}
		v := r.PostFormValue(f.Name)
		if !f.NoTrim {
			v = strings.TrimSpace(v)
		}
		// asked before the value is stored: the list may depend on what the record held
		q := gojiffy.ChoiceQuery{}
		if f.LookupLimit > 0 {
			q.Values = []string{v} // a long list is asked only whether it holds this one
		}
		opts, choice, err := f.choices(r.Context(), store, &item, q)
		if err != nil {
			return item, nil, err
		}
		// empty is what the form's own blank option sends when the field is not Required
		offered := !choice || v == "" && !f.Required || slices.ContainsFunc(opts, func(o view.Option) bool { return o.Value == v })
		if offered && !f.NoSet {
			set(&item, f.Name, v)
		}
		msg := f.check(v)
		if msg == "" && !offered {
			msg = or(f.Errors.Choice, view.T("Choose one of the options"))
		}
		if msg != "" {
			errs[f.Name] = msg
			continue
		}
		if f.Parse != nil {
			if err := f.Parse(&item, v, creating); err != nil {
				errs[f.Name] = err.Error()
			}
		}
	}
	return item, errs, nil
}

// Form builds the form for the template: values out of the record, errors under
// the fields, options for a select out of Choices or, with LookupChoices, out of
// store — nil when the resource has none. A select that is not Required starts
// with a blank option, so that "nothing" can be chosen back. The CSRF token is
// not asked for here — a page puts it into every form it draws, see view.Handler and
// view.RenderPage.
func (rs Resource[T]) Form(ctx context.Context, store gojiffy.Chooser, item T, creating bool, errs map[string]string) (view.FormView, error) {
	v := view.FormView{CancelURL: rs.Path}
	if creating {
		v.Title, v.Submit, v.Action = rs.NewTitle, view.T("Create"), rs.Path
	} else {
		v.Title, v.Submit, v.Action = rs.editTitle(item), view.T("Save"), rs.Href(item)
	}

	for _, f := range rs.Fields {
		if !f.inForm() {
			continue
		}
		fv := view.FieldView{
			Name:     f.Name,
			Label:    f.label(),
			Type:     f.Type,
			Help:     f.Help,
			Autofill: f.Autofill,
			Required: f.Required,
			Error:    errs[f.Name],
		}
		if !creating && f.HelpEdit != "" {
			fv.Help = f.HelpEdit
		}
		if f.Readonly != nil {
			fv.Readonly = f.Readonly(item)
		}
		switch {
		case f.Value != nil:
			fv.Value = f.Value(item)
		case f.Text != nil:
			fv.Value = f.Text(item)
		}
		if f.LookupChoices && f.LookupLimit > 0 {
			fv.Lookup = rs.lookupURL(f)
			opts, _, err := f.choices(ctx, store, &item, gojiffy.ChoiceQuery{Values: []string{fv.Value}})
			if err != nil {
				return view.FormView{}, err
			}
			// a value the store no longer offers — a deleted author — shows as
			// nothing chosen, the way a select shows it
			if i := slices.IndexFunc(opts, func(o view.Option) bool { return o.Value == fv.Value }); i >= 0 {
				fv.ValueText = opts[i].Label
			} else {
				fv.Value = ""
			}
			v.Fields = append(v.Fields, fv)
			continue
		}
		opts, choice, err := f.choices(ctx, store, &item, gojiffy.ChoiceQuery{})
		if err != nil {
			return view.FormView{}, err
		}
		if choice {
			if !f.Required {
				opts = append([]view.Option{{Label: view.T("— none —")}}, opts...)
			}
			fv.Options = selected(opts, fv.Value)
			fv.Value = ""
			// a <select> knows no readonly: the one option it may offer is the value it holds
			if fv.Readonly {
				fv.Options = slices.DeleteFunc(fv.Options, func(o view.Option) bool { return !o.Selected })
			}
		}
		v.Fields = append(v.Fields, fv)
	}
	return v, nil
}

// RenderForm draws the form of a record as a page of its own: the heading of
// the form is the page title, and the crumbs lead back to the list. A form that
// comes back with errors answers 422, so a script or a test sees the failure
// without reading the page. A page that needs more — a notice above, buttons
// beside, crumbs of its own — builds it out of Form, FormBlock and view.RenderPage.
//
//	rs.RenderForm(w, r, app.Frame, db.Articles(), article, creating, errs)
func (rs Resource[T]) RenderForm(w http.ResponseWriter, r *http.Request, frame view.Frame,
	store gojiffy.Chooser, item T, creating bool, errs map[string]string) {

	form, err := rs.Form(r.Context(), store, item, creating, errs)
	if err != nil {
		serverError(w, err)
		return
	}
	status := http.StatusOK
	if len(errs) > 0 {
		status = http.StatusUnprocessableEntity
	}
	view.RenderPage(w, r, frame, status, "", nil, rs.FormBlock(form))
}

// FormBlock puts a form on a page, panel and heading included, with the trail
// through the list to it.
func (rs Resource[T]) FormBlock(form view.FormView) view.Block {
	return view.Block{
		Name:   "form-panel",
		Title:  form.Title,
		Crumbs: slices.Concat(rs.Crumbs, []view.Link{{Title: rs.Title, Href: rs.Path}, {Title: form.Title}}),
		Data:   form,
	}
}
