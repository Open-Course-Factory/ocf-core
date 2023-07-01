title:GIT - TP Rebase interactif
intro:nous permettra de manipuler le rebase interactif.
conclusion:Vu comment utiliser le rebase interactif

---

### Bonus : refaire l'histoire avec git rebase -i

- Cette année Bob s'organise, il se fait des fiches de révision pour son examen d'histoire. Du coup, en bon programmeur, il décide de les rédiger en markdown pour pouvoir les enrichir avec des illustrations et autres bricoles.
- Il a ainsi rédigé 5 fichiers différents pour 5 périodes différentes :
  - la révolution industrielle (la_revolution_industrielle.md) ;
  - la Première Guerre mondiale (la_premiere_guerre_mondiale.md) ;
  - l'entre-deux-guerres (entre_guerre.md) ;
  - la Seconde Guerre mondiale (la_seconde_guerre_mondiale.md) ;
  - les Trente Glorieuses (30_glorieuses.md).

---

- Seulement il y a un problème, Bob a été malade pendant l'année. Du coup, ses fichiers ont été écrits dans le désordre : il a rédigé ses cours au fur et à mesure puis, quand il avait du temps, il a fait du rattrapage en prenant les cours d'Alice.
- Il se retrouve alors avec l'historique de commits git suivant :
  1. 218bfcb Conclusion de la première guerre mondiale - rattrapage
  2. b1cc9fd L'entre deux guerres
  3. 70c08d1 Début de la seconde guerre mondiale - rattrapage
  4. ce99651 Les 30 glorieuses
  5. 61f6523 La seconde guerre mondiale - fin
  6. b52fa42 La première guerre mondiale
  7. 8c310df La révolution industrielle
  8. 47901da Ajout des titres des parties
  9. 5b56868 Creation des fichiers du programme d'histoire

---

- C'est cet historique que nous allons essayer de réordonner. On peut voir que les premiers commits correspondent au début du programme scolaire, puis ensuite Bob a été malade. Il a alors fait une pause, puis repris le cours dans l'ordre. Ensuite, il a rattrapé les cours manquants en recopiant les notes d'un autre élève, en partant du plus récent dans le programme au plus vieux.
- L'historique montré est antéchronologique, le commit le plus récent (celui que l'on vient de faire) est en haut de la liste, le plus vieux est en bas.

---

- Le dépôt git de ce tp est présent dans le fichier zip tuto_git_rebase.zip.
- Dans ce dépôt, vous devez trouver deux branches qui ont pour noms Bob et Alice. La branche Bob possède les écrits de Bob, tandis que celle d'Alice possède quelques anecdotes qu'elle a voulu lui donner plus tard. Pour l'instant, concentrez-vous sur la branche de Bob.

```shell
git checkout Bob
```

- Vous pouvez alors vérifier les commits et leur mauvais rangement. Vous devriez obtenir la liste que l'on a vue plus tôt.

---

#### Les objectifs

- Nos objectifs seront donc multiples. Afin de conserver un dépôt propre, nous allons effectuer une à une les opérations suivantes :
  - mise dans l'ordre chronologique du cours des différents fichiers
  - fusion des commits consécutifs traitant du même fichier et de la même partie "logique"
  - fusion de la branche d'Alice pour "enrichir" notre branche de son contenu

---

#### Déplacer des commits

- À l'heure actuelle, on a un historique un peu farfelu. Dans l'idéal, on aimerait que les éléments se suivent chronologiquement, voire que l'on ait uniquement un seul commit par période.

---

- Ainsi, on va essayer d'obtenir le résultat suivant :

```shell
----------- AVANT -----------
218bfcb Conclusion de la première guerre mondiale - rattrapage
b1cc9fd L'entre deux guerres
70c08d1 Début de la seconde guerre mondiale - rattrapage
ce99651 Les 30 glorieuses
61f6523 La seconde guerre mondiale - fin
b52fa42 La première guerre mondiale
8c310df La révolution industrielle
47901da Ajout des titres des parties
5b56868 Creation des fichiers du programme d'histoire

----------- APRES -----------
ce99651 Les 30 glorieuses
61f6523 La seconde guerre mondiale - fin
70c08d1 Début de la seconde guerre mondiale - rattrapage
b1cc9fd L'entre deux guerres
218bfcb Conclusion de la première guerre mondiale - rattrapage
b52fa42 La première guerre mondiale
8c310df La révolution industrielle
47901da Ajout des titres des parties
5b56868 Creation des fichiers du programme d'histoire
```

---

- Comme vous pouvez le constater, de nombreux commits ont littéralement changé de place ! C'est ça que nous allons faire ici, déplacer des commits !
Et aussi impressionnant que cela puisse paraître, il va suffire d'utiliser une seule commande à bon escient pour le faire : **git rebase**. Mais attention, on ne l'utilise pas n'importe comment.
- Pour l'utiliser, on va devoir lui spécifier le commit le plus ancien devant rester tel quel. Dans notre cas, nous souhaitons tout remettre en ordre jusqu'à "La premiere guerre mondiale" (**b52fa42**) qui, lui, ne bouge pas. On va alors lancer le rebase en mode interactif jusqu'à ce commit :

```shell
git rebase -i b52fa42^
```

- Attention à ne pas oublier l'option **-i** pour le mode interactif ainsi que le **^** après l'identifiant du commit ! Ce dernier sert à indiquer que l'on veut remonter jusqu'à ce commit inclus.

---

- Une nouvelle fenêtre s'ouvre alors avec plein de choses passionnantes :

```shell
pick b52fa42 La première guerre mondiale
pick 61f6523 La seconde guerre mondiale - fin
pick ce99651 Les 30 glorieuses
pick 70c08d1 Début de la seconde guerre mondiale - rattrapage
pick b1cc9fd L'entre deux guerres
pick 218bfcb Conclusion de la première guerre mondiale - rattrapage

# Rebase 8c310df..218bfcb onto 8c310df
#
# Commands:
#  p, pick = use commit
#  r, reword = use commit, but edit the commit message
#  e, edit = use commit, but stop for amending
#  s, squash = use commit, but meld into previous commit
#  f, fixup = like "squash", but discard this commit's log message
#  x, exec = run command (the rest of the line) using shell
#
# If you remove a line here THAT COMMIT WILL BE LOST.
# However, if you remove everything, the rebase will be aborted.
```

---

- Dans cet affichage, vous avez la liste des commits jusqu'au dernier que vous souhaitez garder tel quel. L'opération est maintenant simple, il va falloir déplacer les lignes pour les mettre dans l'ordre que vous voulez. 
- L'ordre en question sera celui que l'on a vu juste au-dessus. Laissez les "**pick**" en début de ligne, ils sont là pour signifier que vous souhaitez utiliser le commit en question.
- Vous devriez obtenir quelque chose comme ça avant de valider :

```shell
pick b52fa42 La première guerre mondiale
pick 218bfcb Conclusion de la première guerre mondiale - rattrapage
pick b1cc9fd L'entre deux guerres
pick 70c08d1 Début de la seconde guerre mondiale - rattrapage
pick 61f6523 La seconde guerre mondiale - fin
pick ce99651 Les 30 glorieuses

# Et en dessous le blabla précédent
```

---

- Sauvegardez puis quittez l'éditeur. Le rebase se lance alors automatiquement… et vous crie dessus, c'est un échec ! 
- Si vous utilisez la commande git status vous allez voir qu'il existe un conflit sur le fichier "la_seconde_guerre_mondiale.md". En l'ouvrant, vous verrez des marqueurs **<<<<<<,** **======** et **>>>>>** que git a rajoutés pour vous signaler les endroits où il n'arrive pas à faire une chose logique. 
- C'est donc à vous de jouer en éditant manuellement le fichier, pour qu'il ait l'allure escomptée. En l'occurrence, c'est simplement une ligne blanche qui l'ennuie. Supprimez-là, ainsi que les marqueurs, puis sauvegarder le fichier.

---

- Nous allons maintenant signaler à git que le conflit est résolu en faisant un :

```shell
git add la_seconde_guerre_mondiale.md
```

- Cela nous permet de rajouter le fichier dans l'index, puis on lui demande de continuer le rebase avec :

```shell
git rebase --continue
```

- Git vous demandera alors de confirmer le message de commit (ce que vous ferez), puis continuera son de chemin.
- Un autre conflit similaire apparaît alors, résolvez-le de la même manière.
- À la fin, git doit afficher le message Successfully rebased and updated refs/heads/Bob. pour nous informer que tout va bien.
- Si vous ré-affichez votre historique, vos commits sont maintenant dans l'ordre !

---

#### Fusionner des commits

- Cette fois-ci nous allons fusionner des commits pour réduire ces derniers, et surtout les rendre cohérents !

---

- On va donc chercher à atteindre le schéma suivant :

```shell
-------- AVANT --------
56701fc Les 30 glorieuses
a63009c Début de la seconde guerre mondiale - rattrapage
752c8cd L'entre deux guerres
328d49e Conclusion de la première guerre mondiale - rattrapage
b52fa42 La première guerre mondiale
8c310df La révolution industrielle
47901da Ajout des titres des parties
5b56868 Creation des fichiers du programme d'histoire

-------- APRES --------
d55d7d3 Les 30 glorieuses
3107653 La seconde guerre mondiale
ca137f6 L'entre deux guerres
ebfa63b La première guerre mondiale
8c310df La révolution industrielle
47901da Ajout des titres des parties
5b56868 Creation des fichiers du programme d'histoire
```

---

- Nous allons aussi en profiter pour mettre à jour un message de commit !
- Là encore, c'est la magie de la commande rebase qui va nous être utile. Comme précédemment, on va la lancer sur le dernier commit qui ne bouge pas, donc **b52fa42** La première guerre mondiale. Ce qui nous donne :

```shell
git rebase -i b52fa42^
```

---

- La machine se met en route et nous affiche le menu permettant de faire les modifications. 
- Nous allons cette fois-ci lui dire de fusionner le commit **328d49e** avec son prédécesseur tout en éditant le message de commit de **a63009c**. On utilisera pour cela "**fixup**" ou "**squash**" pour fusionner (le dernier permet de changer le message de commit lors de la fusion), et nous utiliserons "**reword**" pour éditer juste le message du second commit à modifier.
- Voici la séquence :

```shell
pick b52fa42 La première guerre mondiale
fixup 328d49e Conclusion de la première guerre mondiale - rattrapage
pick 752c8cd L'entre deux guerres
reword a63009c Début de la seconde guerre mondiale - rattrapage
pick 56701fc Les 30 glorieuses
```
- Sauvegardez, quittez, puis laissez la magie opérer ! Lors du processus, l'éditeur devrait apparaître pour vous demander le nouveau commit pour l'opération de "**reword**".

---

#### Rebase une autre branche

- Une dernière fonction bien pratique de l'outil rebase est la fusion entre des branches. Ainsi, si vous travaillez sur une branche pour développer quelque chose, mais que vous voulez récupérer le contenu d'une autre branche pour mettre à jour la vôtre (vous synchroniser avec master par exemple), rebase peut vous y aider.
- Cette fois-ci, on va se servir de rebase non pas en indiquant un commit mais en indiquant la branche que l'on aimerait récupérer dans notre travail. En l'occurrence, on va chercher à récupérer les modifications d'Alice (branche du même nom) qui a pris notre cours puis y a rajouté quelques anecdotes dans son coin.
- Voici la petite commande à lancer :

```shell
git rebase Alice
```

---

- Cette fois-ci, pas besoin du mode "interactif" -i.
- Évidemment, il peut arriver que des conflits se présentent, comme dans ce cas précis. 
- Essayez de les corriger avec l'éditeur, puis il suffit de faire un `git add <lefichier>` suivi d'un `git rebase --continue` pour continuer le rebase !
- Vous voilà maintenant synchronisés avec la branche d'Alice. 
