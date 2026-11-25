package story

import (
	"context"
	"path/filepath"
	"testing"

	"showmethestory/internal/config"
)

func TestDeskSettingSuggestionsReviewAndUndo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "progress.json")
	state := &Progress{Desk: StoryDesk{Parts: []DeskPart{{DeskDraft: DeskDraft{Beats: "Lisa is Dave's mom. The station closes at midnight."}}}}}
	settings := &ProjectSettings{
		Characters: []Character{{ID: "c_1", Name: "Lisa", Background: "Retired engineer."}, {ID: "c_2", Name: "Dave"}},
		Worldview:  []WorldviewEntry{{ID: "w_1", Name: "Station", Category: "geography", Description: "Closes at ten."}},
	}
	model := batchAPI(t, func(prompt string) string {
		return `{"title":"The Station","lines":["Lisa is Dave's mom. The station closes at midnight. A blue lamp hangs in the hall."],"setting_suggestions":[
			{"kind":"characters","entity_id":"c_1","field":"background","after":"Retired engineer. Dave's mother.","beat":1,"origin":"draft","quote":"Lisa is Dave's mom"},
			{"kind":"relations","source_id":"c_1","target_id":"c_2","after":"mother of","beat":1,"origin":"draft","quote":"Lisa is Dave's mom"},
			{"kind":"worldview","entity_id":"w_1","field":"description","after":"Closes at midnight.","beat":1,"origin":"draft","quote":"The station closes at midnight"},
			{"kind":"worldview","name":"Blue lamp","category":"other","after":"In the station hall.","beat":1,"origin":"outline","quote":"A blue lamp hangs in the hall"},
			{"kind":"characters","name":"Sam","after":"Friend.","beat":1,"origin":"draft","quote":"invented evidence"},
			{"kind":"characters","name":"Lisa","after":"Duplicate.","beat":1,"origin":"draft","quote":"Lisa is Dave's mom"}
		]}`
	})
	cfg := config.DefaultConfigForLang("en")
	run := func(action string, index int, value *string) error {
		_, err := RunDesk(context.Background(), path, state, cfg, settings, nil, model, DeskRequest{
			Action: action, Part: 1, Revision: state.Desk.Revision, Version: 1, Suggestion: index, SettingValue: value,
		})
		return err
	}
	if err := run("generate", 0, nil); err != nil {
		t.Fatal(err)
	}
	created := state.Desk.Parts[0].Versions[0].SettingSuggestions
	if len(created) != 4 || created[0].Before != "Retired engineer." || created[3].Origin != "outline" {
		t.Fatalf("suggestions = %#v", created)
	}
	if settings.Characters[0].Background != "Retired engineer." || len(settings.Relations) != 0 {
		t.Fatal("generation changed settings before review")
	}
	edited := "Retired engineer. Mother of Dave."
	for _, input := range []struct {
		action string
		index  int
		value  *string
	}{
		{"apply_setting", 1, &edited}, {"apply_setting", 1, &edited},
		{"apply_setting", 2, nil}, {"dismiss_setting", 4, nil},
	} {
		if err := run(input.action, input.index, input.value); err != nil {
			t.Fatal(err)
		}
	}
	if settings.Characters[0].Background != edited || len(settings.Relations) != 1 || len(settings.StoryChanges) != 3 {
		t.Fatalf("settings after review = %#v", settings)
	}
	settings.Worldview[0].Description = "Author override."
	if err := run("apply_setting", 3, nil); err == nil {
		t.Fatal("stale worldview change overwrote an author edit")
	}
	settings.Characters[0].Notes = "Keep this newer note."
	invalidateSettingSources(settings, state)
	if err := run("undo_setting", 1, nil); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadProjectSettings(filepath.Join(filepath.Dir(path), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Characters[0].Background != "Retired engineer." || reloaded.Characters[0].Notes != "Keep this newer note." ||
		deskSettingDecisions(reloaded)["1:1:1"] != "reverted" || deskSettingDecisions(reloaded)["1:1:4"] != "rejected" {
		t.Fatalf("settings after undo = %#v", reloaded)
	}
}
