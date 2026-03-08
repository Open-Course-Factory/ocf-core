# Makefile pour les tests de l'entity manager

.PHONY: test test-unit test-integration test-entity-manager clean coverage help

# Variables
COVERAGE_OUT = coverage.out
COVERAGE_HTML = coverage.html

# Commande par défaut
help:
	@echo "Commandes disponibles pour les tests de l'Entity Manager:"
	@echo ""
	@echo "  test              - Lance tous les tests"
	@echo "  test-unit         - Lance seulement les tests unitaires"
	@echo "  test-integration  - Lance seulement les tests d'intégration"
	@echo "  test-entity-manager - Lance seulement les tests de l'entity manager"
	@echo "  coverage          - Génère un rapport de couverture"
	@echo "  coverage-html     - Génère un rapport de couverture HTML"
	@echo "  benchmark         - Lance les benchmarks"
	@echo "  clean            - Nettoie les fichiers de test"
	@echo ""

# Tests complets
test:
	@echo "🧪 Lancement de tous les tests..."
	go test -v ./tests/... ./src/... -race -timeout=120s

# Tests unitaires uniquement
test-unit:
	@echo "🔬 Lancement des tests unitaires..."
	go test -v ./tests/... ./src/... -timeout=120s -short

# Tests d'intégration
test-integration:
	@echo "🔗 Lancement des tests d'intégration..."
	go test -v ./tests/courses/... -race -timeout=60s

# Tests de l'entity manager spécifiquement
test-entity-manager:
	@echo "⚙️  Lancement des tests de l'Entity Manager..."
	@echo "📦 Compiling tests..."
	@go test -c -o entityManagement_tests.test ./tests/entityManagement
	@echo "🧪 Running compiled tests..."
	@./entityManagement_tests.test -test.v -test.timeout=30s
	@rm -f entityManagement_tests.test

# Couverture de code
coverage:
	@echo "📊 Génération du rapport de couverture..."
	go test -coverprofile=$(COVERAGE_OUT) ./tests/entityManagement/...
	go tool cover -func=$(COVERAGE_OUT)

# Couverture de code avec HTML
coverage-html: coverage
	@echo "🌐 Génération du rapport de couverture HTML..."
	go tool cover -html=$(COVERAGE_OUT) -o $(COVERAGE_HTML)
	@echo "📂 Rapport disponible dans $(COVERAGE_HTML)"

# Benchmarks
benchmark:
	@echo "🏃‍♂️ Lancement des benchmarks..."
	go test -bench=. -benchmem ./tests/entityManagement/... -timeout=60s

# Tests avec mode verbose et détails
test-verbose:
	@echo "📝 Tests détaillés de l'Entity Manager..."
	go test -v -race -cover ./tests/entityManagement/... -timeout=30s

# Test spécifique à un service
test-registration:
	go test -v ./tests/entityManagement/entityRegistrationService_test.go -race

test-service:
	go test -v ./tests/entityManagement/genericService_test.go -race

test-repository:
	go test -v ./tests/entityManagement/genericRepository_test.go -race

test-controller:
	go test -v ./tests/entityManagement/genericController_test.go -race

# Nettoyage
clean:
	@echo "🧹 Nettoyage des fichiers de test..."
	rm -f $(COVERAGE_OUT) $(COVERAGE_HTML)
	go clean -testcache

# Tests en mode watch (nécessite air ou equivalent)
test-watch:
	@echo "👀 Mode watch des tests (Ctrl+C pour arrêter)..."
	@which air > /dev/null || (echo "❌ 'air' n'est pas installé. Installez-le avec: go install github.com/cosmtrek/air@latest" && exit 1)
	air -c .air-test.toml

# Vérification de la qualité du code
lint:
	@echo "🔍 Vérification du code avec golangci-lint..."
	@which golangci-lint > /dev/null || (echo "❌ 'golangci-lint' n'est pas installé" && exit 1)
	golangci-lint run ./tests/entityManagement/...

# Tests de race conditions spécifiquement
test-race:
	@echo "🏁 Test des race conditions..."
	go test -race -count=5 ./tests/entityManagement/...

# Profiling mémoire
test-profile:
	@echo "📈 Profiling des tests..."
	go test -memprofile=mem.prof -cpuprofile=cpu.prof ./tests/entityManagement/...
	@echo "Profiles générés: mem.prof, cpu.prof"

# Tests parallèles avec différents paramètres
test-parallel:
	@echo "⚡ Tests parallèles..."
	go test -parallel=4 -race ./tests/entityManagement/...

# Validation complète avant commit
pre-commit: clean test-unit coverage lint
	@echo "✅ Validation pré-commit terminée avec succès!"

# Installation des outils nécessaires
install-tools:
	@echo "🛠️  Installation des outils de développement..."
	go install github.com/air-verse/air@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Affichage des statistiques de test
test-stats:
	@echo "📊 Statistiques des tests..."
	@find tests/entityManagement -name "*_test.go" -exec wc -l {} + | tail -1 | awk '{print "Lignes de tests:", $$1}'
	@find tests/entityManagement -name "*_test.go" -exec grep -c "^func Test" {} + | awk '{sum+=$$1} END {print "Fonctions de test:", sum}'
	@find tests/entityManagement -name "*_test.go" -exec grep -c "^func Benchmark" {} + | awk '{sum+=$$1} END {print "Benchmarks:", sum}'

# Mode debug avec plus d'informations
test-debug:
	@echo "🐛 Tests en mode debug..."
	go test -v -race -cover -timeout=60s ./tests/entityManagement/... -args -test.v=true