# Cours Golang Débutants

Ce cours est prévu pour être importé dans OCF (Open Course Factory).
Le dépôt contient tous les éléments nécessaires à l'import d'un cours.


Lancer l'image qui permet d'exporter :

```shell
docker run --name slidev_export --rm -dit     -v ${PWD}:/slidev     -p 3031:3030     -e NPM_MIRROR="https://registry.npmmirror.com"     tangramor/slidev:playwright
```

Installer Playwright à l'intérieur : 

```shell
docker exec -i slidev_export npx playwright install
```

Builder les slides en SPA :

```shell
docker exec -i slidev_export npx slidev build
```

Run a container embedding the SPA : 

```shell
docker run -d --name myslides --rm -p 3030:80 -v ${PWD}/dist:/usr/share/nginx/html nginx:alpine
```