title:Les branches, des pointeurs
intro:nous expliquera comment fonctionnent les branches.
conclusion:Découvert que les branches ne sont que des pointeurs.

---

### HEAD, tête de lecture

![center w60](images/git_head-to-master.png)

---

```shell
$ git branch testing
```

![center w60](images/git_head-to-master-testing.png)

---

```shell
$ git checkout testing
```

![center w60](images/git_head-to-testing.png)

---

```shell
$ vim fichier #ici on fait des changements
$ git commit -m 'changements dans le fichier !'
```

![center w80](images/git_advance-testing.png)

---

```shell
$ git checkout master
```

![center w80](images/git_checkout-master.png)

---

```shell
$ git merge testing
```

![center w80](images/git_merge-testing.png)
