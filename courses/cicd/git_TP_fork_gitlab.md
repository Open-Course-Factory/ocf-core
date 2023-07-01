title:GIT - TP Fork - Gitlab
intro:nous permettra de comprendre le fork et les manipulation de base.
conclusion:Vu ce qu'est un fork

---

## Pré-requis

- Git installé
- Un compte sur https://gitlab.com

---

## Fork d'un dépôt Gitlab

- Aller sur l'adresse suivante et faire un fork du dépôt (en haut à droite) : https://gitlab.com/tsaquet/first-day-c
- Copier l'url du nouveau dépôt créé (Bouton "Clone" sur la droite, choisir "with HTTPS") et faire un clone de celui-ci en local.

1. Placez-vous sur le dossier racine de ce dépôt puis afficher les branches locales présentes.
2. Affichez maintenant l'ensemble des branches (locales + distantes) du dépôt, que remarquez-vous ?
3. Créez une branche locale 'go' qui suivra une branche distante nommée également 'go'
4. Affichez de nouveau la liste des branches, où êtes-vous ?

---

5. Déplacez-vous sur la nouvelle branche.
6. Vérifiez que vous êtes bien sur la branche  
   souhaitée.
7. Revenez sur la branche master
8. Créez une nouvelle branche locale et placez-vous dessus directement avec une seule commande
9. Créez un nouveau fichier
10. Vérifiez l'état de votre espace de travail
11. Quel est l'état de votre fichier ?
12. Ajoutez ce fichier à l'index git
13. Quel est le nouvel état du fichier ?
14. Créez un nouveau commit.

---

15. Quel est maintenant l'état de votre espace de travail ?
16. Modifiez le contenu du fichier existant et créez également un nouveau fichier
17. Essayez de faire un nouveau commit, que se passe-t-il ?
18. Essayez de nouveau avec la commande suivante :

```shell
git commit -am "blabla"
```
19. Que s'est-il passé selon vous ? Quel est l'état de votre répertoire de travail ?
20. Ignorez le dernier fichier créé, en ajoutant le nom de ce fichier dans le fichier .gitignore

---

21. Vérifiez que ce fichier est bien ignoré  
    (avec la commande git status)
22. Affichez l'historique des derniers commits.
23. Utilisez la commande git ci-dessous pour vous créer un alias git affichant les commits comme un graphe :

```shell
git config --global alias.lg \
  "log --graph \
  --pretty=format:'%Cgreen%h%Creset -%Creset %s%C(yellow)%d %Cblue(%aN, %cr)%Creset' \
  --abbrev-commit --date=relative"
```

---

24. Utilisez cette nouvelle commande pour visualiser les commits sous forme de graphe :

```shell
git lg 
```

25. Affichez l'ensemble des branches (locales et distantes), que remarquez-vous ?
26. Ajoutez votre branche au dépôt distant
27. Affichez de nouveau l'ensemble des branches pour vérifier qu'elle a bien été ajoutée. Vous pouvez également aller sur votre page Gitlab pour voir l'état du dépôt distant
