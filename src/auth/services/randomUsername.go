package services

import (
	"math/rand"
	"strconv"
)

var usernameAdjectives = []string{
	"agile", "bold", "brave", "bright", "calm", "clever", "cosmic", "curious",
	"eager", "fancy", "gentle", "happy", "jolly", "keen", "kind", "lively",
	"lucky", "merry", "mighty", "nimble", "noble", "patient", "quick", "quiet",
	"sharp", "silent", "sunny", "swift", "vivid", "witty",
}

var usernameNouns = []string{
	"badger", "beaver", "bison", "condor", "cougar", "crane", "dolphin", "falcon",
	"ferret", "gecko", "heron", "ibex", "jaguar", "koala", "lemur", "lynx",
	"marmot", "meerkat", "narwhal", "ocelot", "osprey", "otter", "panda", "puffin",
	"quokka", "raven", "salmon", "tapir", "walrus", "wombat",
}

// randomUsername returns an "adjective_noun<digit>" handle, the shape the
// docker names generator produced before. It is only stored as a Casdoor user
// property, never used as a unique key, so collisions are harmless.
func randomUsername() string {
	return usernameAdjectives[rand.Intn(len(usernameAdjectives))] + "_" +
		usernameNouns[rand.Intn(len(usernameNouns))] + strconv.Itoa(rand.Intn(10))
}
