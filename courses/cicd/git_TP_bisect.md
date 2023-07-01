title:GIT - TP Bisect
intro:nous permettra de manipuler l'outil bisect.
conclusion:Vu un exemple de bisect.

---

## Débusquer rapidement l'origine d'un bug avec git bisect

### Initialisation

- Décompressez le fichier bisect-demo.zip où bon vous semble ; il crée un dossier bisect-demo dans lequel vous n'avez plus qu'à ouvrir une ligne de commande (sous Windows, préférez Git Bash). Ce dépôt contient plus de 1 000 commits répartis sur environ un an et, quelque part là-dedans, un bug s'est glissé.
- En effet, si vous exécutez ./demo.sh, il affiche un KO tout penaud. Alors qu'il devrait afficher glorieusement OK. Ce souci remonte à assez loin, et nous allons utiliser git bisect pour le débusquer.

---

1. Vérifiez que le script ne fonctionne pas sur le dernier commit
2. Démarrez Git bisect à ce commit (bad)
3. Ici on n'a aucune idée du dernier commit valable, alors on va prendre le commit initial, trouvez l'identifiant de ce commit
4. Déplacez-vous sur celui-ci et vérifiez l'exécution du script
5. Identifiez le comme valable (good), git bisect ainsi initialisé va commencer à faire de nombreux checkout 
6. Après chaque checkout, exécuter le script et indiquez si il est bon (good) ou non (bad), jusqu'à trouver le commit problèmatique
7. Quel est l'identifiant de ce commit, sa date ainsi que son auteur ?
8. Terminez le bisect
9. Dans ce cas précis où l'erreur était simplement une modification d'un fichier, trouvez deux autres méthodes simples et rapides pour trouver le commit défectueux (avec une seule commande)
