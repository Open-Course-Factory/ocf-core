package scenarios_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
	"soli/formations/src/scenarios/utils"
)

// fullyPopulatedScenarioInput sets every field the JSON shape carries, already
// in the form the export writes back (normalised step types, sorted locales),
// so that export(import(x)) == x can be asserted directly.
func fullyPopulatedScenarioInput() dto.SeedScenarioInput {
	sessionUser := 1000
	portExposure := true
	return dto.SeedScenarioInput{
		Title:                   "Lossless Castle",
		Description:             "Every field, once",
		Difficulty:              "intermediate",
		EstimatedTimeMinutes:    45,
		InstanceType:            "ocf-castle",
		Hostname:                "castle",
		OsType:                  "deb",
		FlagsEnabled:            true,
		AllowedFlagPaths:        "/home,/srv",
		CrashTraps:              true,
		PortExposureAllowed:     &portExposure,
		SessionUser:             &sessionUser,
		IntroText:               "# Welcome to {{W_CELLAR}}",
		FinishText:              "# Well done",
		SetupScript:             "#!/bin/bash\nmkdir -p \"$P_CELLAR\"",
		Objectives:              "- find the crown",
		Prerequisites:           "- cd and ls",
		DefaultLocale:           "en",
		Locales:                 []string{"en", "fr"},
		CompatibleInstanceTypes: []string{"ocf-castle", "debian-12"},
		RequiredFeatures:        []string{"network"},
		BuildFeatures:           []string{"docker"},
		Translations: []dto.SeedScenarioTranslationInput{{
			Locale: "fr", Title: "Château sans perte", Description: "Chaque champ, une fois",
			Objectives: "- trouver la couronne", Prerequisites: "- cd et ls",
			IntroText: "# Bienvenue", FinishText: "# Bravo",
		}},
		Lexicon: []dto.LexiconEntryInput{
			{Key: "CASTLE", Kind: "place", Names: map[string]string{"en": "Castle", "fr": "Chateau"}},
			{Key: "CELLAR", ParentKey: "CASTLE", Kind: "place", Names: map[string]string{"en": "Cellar", "fr": "Cave"}},
		},
		Steps: []dto.SeedStepInput{
			{
				Title:                    "Go down",
				StepType:                 "terminal",
				TextContent:              "Go to the cellar",
				HintContent:              "### Hint 1\nuse cd\n### Hint 2\ncd \"$P_CELLAR\"",
				VerifyScript:             "#!/bin/bash\n[ \"$PWD\" = \"$P_CELLAR\" ]",
				BackgroundScript:         "#!/bin/bash\ntouch /tmp/ready",
				ForegroundScript:         "cd ~",
				IntroEffect:              "beams",
				IntroText:                "Level 1",
				OutroEffect:              "decrypt",
				OutroText:                "Done",
				BackgroundTimeoutSeconds: 120,
				BackgroundAsync:          true,
				Translations: []dto.SeedStepTranslationInput{{
					Locale: "fr", Title: "Descendre", TextContent: "Va à la cave",
					HintContent: "utilise cd", IntroText: "Niveau 1", OutroText: "Fini",
				}},
			},
			{
				Title:     "Capture",
				StepType:  "flag",
				HasFlag:   true,
				FlagPath:  "/home/learner/flag.txt",
				FlagLevel: 2,
			},
			{
				Title:                 "Check",
				StepType:              "quiz",
				ShowImmediateFeedback: true,
				Questions: []dto.SeedQuestionInput{
					{Order: 0, QuestionText: "Where?", QuestionType: "multiple_choice",
						Options: `["Attic","Cellar"]`, CorrectAnswer: "1", Explanation: "Down", Points: 2},
					{Order: 1, QuestionText: "Root?", QuestionType: "true_false", CorrectAnswer: "false", Points: 1},
					{Order: 2, QuestionText: "Command?", QuestionType: "free_text", CorrectAnswer: "cd", Points: 1},
				},
			},
			{Title: "Read me", StepType: "info", TextContent: "That is all"},
		},
	}
}

// exportedJSON round-trips an export through its wire form, the way a
// teacher's file does.
func exportedJSON(t *testing.T, db *gorm.DB, scenarioID uuid.UUID) dto.SeedScenarioInput {
	t.Helper()
	exported, err := services.NewScenarioExportService(db).ExportAsJSON(scenarioID)
	require.NoError(t, err)
	raw, err := json.Marshal(exported)
	require.NoError(t, err)
	var decoded dto.SeedScenarioInput
	require.NoError(t, json.Unmarshal(raw, &decoded))
	return decoded
}

func TestScenarioJSONExport_ImportedIntoAnotherOrg_ExportsIdentically(t *testing.T) {
	db := freshTestDB(t)
	seeder := services.NewScenarioSeedService(db)
	orgA, orgB := uuid.New(), uuid.New()
	original := fullyPopulatedScenarioInput()

	first, _, err := seeder.SeedScenario(original, "author-a", &orgA)
	require.NoError(t, err)
	exportedFromA := exportedJSON(t, db, first.ID)
	assert.Equal(t, original, exportedFromA, "the export must carry every field the import accepted")

	second, isUpdate, err := seeder.SeedScenario(exportedFromA, "author-b", &orgB)
	require.NoError(t, err)
	require.False(t, isUpdate)
	require.NotEqual(t, first.ID, second.ID)
	assert.Equal(t, exportedFromA, exportedJSON(t, db, second.ID))

	var stored models.Scenario
	require.NoError(t, db.First(&stored, "id = ?", second.ID).Error)
	assert.Equal(t, &orgB, stored.OrganizationID, "ownership comes from the importing route, never the file")
	assert.Equal(t, "author-b", stored.CreatedByID)
}

func TestScenarioJSONExport_CarriesNoSecretOrOwnership(t *testing.T) {
	db := freshTestDB(t)
	orgA := uuid.New()
	seeded, _, err := services.NewScenarioSeedService(db).SeedScenario(fullyPopulatedScenarioInput(), "author-a", &orgA)
	require.NoError(t, err)
	require.NoError(t, db.Model(&models.Scenario{}).Where("id = ?", seeded.ID).
		Updates(map[string]any{"flag_secret": "s3cr3t-value", "is_public": true}).Error)

	exported, err := services.NewScenarioExportService(db).ExportAsJSON(seeded.ID)
	require.NoError(t, err)
	raw, err := json.Marshal(exported)
	require.NoError(t, err)

	body := string(raw)
	assert.NotContains(t, body, "s3cr3t-value")
	for _, key := range []string{`"flag_secret"`, `"organization_id"`, `"created_by_id"`, `"is_public"`, `"id"`, `"source_hash"`} {
		assert.NotContains(t, body, key)
	}
}

// A translation's staleness is measured against the step it was written for,
// so an imported one is stamped against the imported step — it must not read
// as stale the moment it lands.
func TestScenarioJSONImport_StampsStepTranslationsAsCurrent(t *testing.T) {
	db := freshTestDB(t)
	org := uuid.New()
	seeded, _, err := services.NewScenarioSeedService(db).SeedScenario(fullyPopulatedScenarioInput(), "author", &org)
	require.NoError(t, err)

	var step models.ScenarioStep
	require.NoError(t, db.Where("scenario_id = ? AND \"order\" = 0", seeded.ID).First(&step).Error)
	var translation models.ScenarioStepTranslation
	require.NoError(t, db.Where("step_id = ? AND locale = ?", step.ID, "fr").First(&translation).Error)
	assert.Equal(t, services.StepSourceHash(step), translation.SourceHash)
}

func TestScenarioJSONImport_AppliesHostname(t *testing.T) {
	db := freshTestDB(t)
	input := fullyPopulatedScenarioInput()
	input.Hostname = "dungeon"

	seeded, _, err := services.NewScenarioSeedService(db).SeedScenario(input, "author", nil)
	require.NoError(t, err)

	var stored models.Scenario
	require.NoError(t, db.First(&stored, "id = ?", seeded.ID).Error)
	assert.Equal(t, "dungeon", stored.Hostname)
}

// Re-seeding with a file that predates a field must not wipe what the editor
// set; the challenges seeder sends none of these.
func TestScenarioJSONReimport_WithoutOptionalFields_KeepsThem(t *testing.T) {
	db := freshTestDB(t)
	seeder := services.NewScenarioSeedService(db)
	org := uuid.New()
	full := fullyPopulatedScenarioInput()
	seeded, _, err := seeder.SeedScenario(full, "author", &org)
	require.NoError(t, err)

	bare := full
	bare.Hostname, bare.Objectives, bare.Prerequisites, bare.DefaultLocale = "", "", "", ""
	bare.Locales, bare.PortExposureAllowed, bare.Translations, bare.Lexicon = nil, nil, nil, nil
	bare.Steps = append([]dto.SeedStepInput(nil), full.Steps...)
	bare.Steps[0].Translations = nil
	_, isUpdate, err := seeder.SeedScenario(bare, "author", &org)
	require.NoError(t, err)
	require.True(t, isUpdate)

	assert.Equal(t, full, exportedJSON(t, db, seeded.ID))
}

// The archive carries less than the JSON — no translations, lexicon, locales,
// objectives, prerequisites, os_type, allowed_flag_paths or flag_level — but
// everything it does carry must come back.
func TestScenarioArchiveExport_ImportedIntoAnotherOrg_KeepsWhatTheArchiveCarries(t *testing.T) {
	db := freshTestDB(t)
	orgA, orgB := uuid.New(), uuid.New()
	original := fullyPopulatedScenarioInput()
	seeded, _, err := services.NewScenarioSeedService(db).SeedScenario(original, "author-a", &orgA)
	require.NoError(t, err)

	zipBytes, _, err := services.NewScenarioExportService(db).ExportAsArchive(seeded.ID)
	require.NoError(t, err)
	archiveDir := extractArchiveForTest(t, zipBytes)

	imported, err := services.NewScenarioImporterService(db).ImportFromDirectory(archiveDir, "author-b", &orgB, "upload")
	require.NoError(t, err)

	want := withoutJSONOnlyFields(original)
	got := withoutJSONOnlyFields(exportedJSON(t, db, imported.ID))
	assert.Equal(t, want, got)
}

func withoutJSONOnlyFields(in dto.SeedScenarioInput) dto.SeedScenarioInput {
	out := in
	out.Translations, out.Lexicon, out.Locales = nil, nil, nil
	out.DefaultLocale, out.Objectives, out.Prerequisites, out.OsType, out.AllowedFlagPaths = "", "", "", "", ""
	out.Steps = make([]dto.SeedStepInput, len(in.Steps))
	for i, step := range in.Steps {
		step.Translations = nil
		step.FlagLevel = 0
		out.Steps[i] = step
	}
	return out
}

func extractArchiveForTest(t *testing.T, zipBytes []byte) string {
	t.Helper()
	_, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	require.NoError(t, err)
	archivePath := filepath.Join(t.TempDir(), "scenario.zip")
	require.NoError(t, os.WriteFile(archivePath, zipBytes, 0o644))
	dir := t.TempDir()
	require.NoError(t, utils.ExtractArchive(archivePath, dir))
	scenarioDir, err := utils.FindIndexJSON(dir)
	require.NoError(t, err)
	return scenarioDir
}

// A platform import looks for its namesake among platform scenarios only: an
// organisation's scenario with the same title is someone else's and must not
// be overwritten.
func TestPlatformImport_SameTitleAsAnOrgScenario_CreatesASeparateOne(t *testing.T) {
	db := freshTestDB(t)
	org := uuid.New()
	input := fullyPopulatedScenarioInput()
	orgScenario, _, err := services.NewScenarioSeedService(db).SeedScenario(input, "org-author", &org)
	require.NoError(t, err)

	platformJSON, isUpdate, err := services.NewScenarioSeedService(db).SeedScenario(input, "admin", nil)
	require.NoError(t, err)
	assert.False(t, isUpdate)
	assert.NotEqual(t, orgScenario.ID, platformJSON.ID)

	zipBytes, _, err := services.NewScenarioExportService(db).ExportAsArchive(orgScenario.ID)
	require.NoError(t, err)
	db.Unscoped().Delete(&models.Scenario{}, "id = ?", platformJSON.ID)
	platformArchive, err := services.NewScenarioImporterService(db).ImportFromDirectory(extractArchiveForTest(t, zipBytes), "admin", nil, "upload")
	require.NoError(t, err)
	assert.NotEqual(t, orgScenario.ID, platformArchive.ID)

	var stored models.Scenario
	require.NoError(t, db.First(&stored, "id = ?", orgScenario.ID).Error)
	assert.Equal(t, &org, stored.OrganizationID)
	assert.Equal(t, "org-author", stored.CreatedByID)
	assert.Nil(t, platformArchive.OrganizationID)
}
