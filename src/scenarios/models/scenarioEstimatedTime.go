package models

import (
	"log"
	"regexp"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

// The prose that used to live in estimated_time, in the three spellings the
// data actually contained: "90 minutes", "10m", and empty. An hour is admitted
// too — the column's own comment offered "1h" as an example, so a row somewhere
// may well have taken it up.
var legacyDuration = regexp.MustCompile(`(?i)(\d+)\s*(h|hour|hours|heure|heures|m|min|minute|minutes)?`)

// ParseLegacyEstimatedTime reads a duration written for a person and returns
// minutes. Anything it cannot read is zero, which is what an unset estimate
// already looked like — the field has always been decoration, and guessing
// would put a number in front of a learner that nobody wrote.
func ParseLegacyEstimatedTime(text string) int {
	match := legacyDuration.FindStringSubmatch(strings.TrimSpace(text))
	if match == nil {
		return 0
	}

	value, err := strconv.Atoi(match[1])
	if err != nil {
		return 0
	}

	if strings.HasPrefix(strings.ToLower(match[2]), "h") {
		return value * 60
	}
	return value
}

// MigrateEstimatedTimeToMinutes moves the old prose column into the new integer
// one and then removes it.
//
// Dropping it is the point rather than a tidy-up: two columns holding the same
// estimate is how one of them goes stale, and the reader cannot tell which one
// the product believes. The value is not lost — it is carried across first, and
// this runs only while the old column is still there.
func MigrateEstimatedTimeToMinutes(db *gorm.DB) {
	if !db.Migrator().HasColumn(&Scenario{}, "estimated_time") {
		return
	}

	// The field name has to match the old column, not the new one: this struct
	// exists to read what is being migrated away from.
	type legacyRow struct {
		ID            string
		EstimatedTime string
	}
	var rows []legacyRow
	if err := db.Model(&Scenario{}).
		Select("id, estimated_time").
		Where("estimated_time IS NOT NULL AND estimated_time <> ''").
		Scan(&rows).Error; err != nil {
		log.Printf("could not read the old estimated_time column: %v", err)
		return
	}

	for _, row := range rows {
		minutes := ParseLegacyEstimatedTime(row.EstimatedTime)
		if minutes == 0 {
			continue
		}
		if err := db.Model(&Scenario{}).Where("id = ?", row.ID).
			Update("estimated_time_minutes", minutes).Error; err != nil {
			log.Printf("could not carry %q across for scenario %s: %v", row.EstimatedTime, row.ID, err)
			return
		}
	}

	if err := db.Migrator().DropColumn(&Scenario{}, "estimated_time"); err != nil {
		log.Printf("could not drop the old estimated_time column: %v", err)
		return
	}

	// Ask rather than trust: GORM's SQLite driver returns no error from
	// DropColumn and leaves the column exactly where it was. A no-op that
	// reports success is worse than a failure, because the next run reads the
	// stale column again and quietly re-migrates from it.
	if db.Migrator().HasColumn(&Scenario{}, "estimated_time") {
		log.Printf("the old estimated_time column survived its drop on %s; "+
			"values were carried across, but the column is still there",
			db.Dialector.Name())
	}
}
