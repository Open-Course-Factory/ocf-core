<!-- markdownlint-disable MD013 MD024 -->

# OCF Core

This is the core part of Open Course Factory. It is an API.

## Features

- Write courses and be able to reuse part of them at will
- Templating system to adapt your courses to whoever you work with
- Adding environments (VMs, Cloud, etc.) to the courses to be able to practice

This product is developped within [Solution Libre](https://www.solution-libre.fr/).

⚠ Currently under development, many things can still change, expect bugs and problems.

## How to start

### Setup casdoor

Casdoor has its own configuration file. It can be found in `/src/configuration/casdoor_app.conf`. Default value should work for test purpose, do not use as it is for production.  

Casdoor can be initialized with a `init_data.json` file. [The documentation can be found here ](https://casdoor.org/docs/deployment/data-initialization/). Not provided (yet).

You have to start Casdoor using the docker compose file provided.

The default login/pwd for casdoor is admin/123.

To make everything work, in casdoor you can use the default data created by casdoor, but we recommand to create specific entities for OCF :
- A certificate
  - Once generated, put the certificate in a file `src/auth/casdoor/token_jwt_key.pem`
- An application
  Non default parameters : 
  - Signin session : true
  - The certificate you created before
  - Organization (the one below, can be set after creation)
- A organization
  - Default application (the one before, can be set after creation)

Once it is done, you can retrieve the application secret and client id you need to put in the `.env` file

### Generate documentation

The API documentation id provided by Swagger. You have to generate it first : 

```shell
swag init --parseDependency --parseInternal
```

### Setup database

Add a default user and password in pgadmin.
Now you can use it to connect to postgres and setup the database.

### Use docker compose to start all needed containers

If you work with vscode and Dev Containers, it will create all the containers you need.

Otherwise, you can start everything with the command:

`docker compose up -d`

## API Documentation - WIP

This part is under active develoment, it is currently testable with help oh the core team, will be fully operational (first SaaS version) for january 2025.

### Database

The database is PostgreSQL. 

### Authentication

Casdoor is responsible for authentication. Casdoor allows an user to log in and it provides a JWT token used to authenticate the user through the API.

#### Connecting Casdoor

The connection between OCF and Casdoor relies on a certificate / public / private key system.
The certificate must be generated with Casdoor and then added to the project in a file `token_jwt_key.pem`.
It will be automatically loaded by the project.

### Start the server

Without arguments, the program starts a web server which is an API.

```shell
go run main.go
[GIN-debug] [WARNING] Creating an Engine instance with the Logger and Recovery middleware already attached.

[GIN-debug] [WARNING] Running in "debug" mode. Switch to "release" mode in production.
 - using env:   export GIN_MODE=release
 - using code:  gin.SetMode(gin.ReleaseMode)

[GIN-debug] GET    /api/v1/courses/          --> soli/formations/src/courses/routes/courseRoutes.CourseController.GetCourses-fm (5 handlers)
[GIN-debug] POST   /api/v1/courses/generate  --> soli/formations/src/courses/routes/courseRoutes.CourseController.GenerateCourse-fm (5 handlers)
[GIN-debug] POST   /api/v1/courses/git       --> soli/formations/src/courses/routes/courseRoutes.CourseController.CreateCourseFromGit-fm (5 handlers)
[GIN-debug] DELETE /api/v1/courses/:id       --> soli/formations/src/courses/routes/courseRoutes.CourseController.DeleteCourse-fm (5 handlers)
[GIN-debug] GET    /api/v1/sessions/         --> soli/formations/src/courses/routes/sessionRoutes.SessionController.GetSessions-fm (5 handlers)
[GIN-debug] DELETE /api/v1/sessions/:id      --> soli/formations/src/courses/routes/sessionRoutes.SessionController.DeleteSession-fm (5 handlers)
[GIN-debug] GET    /api/v1/auth/login        --> soli/formations/src/auth.AuthController.Login-fm (4 handlers)
[GIN-debug] GET    /swagger/*any             --> github.com/swaggo/gin-swagger.CustomWrapHandler.func1 (4 handlers)
[GIN-debug] [WARNING] You trusted all proxies, this is NOT safe. We recommend you to set a value.
Please check https://pkg.go.dev/github.com/gin-gonic/gin#readme-don-t-trust-all-proxies for details.
[GIN-debug] Listening and serving HTTP on :8000
```

### Swagger

With the default debug configuration, a Swagger documentation is available at:

```shell
http://localhost:8080/swagger/index.html
```

### Permissions

Permission system relies on Casbin which is provided by Casdoor.

You can find the rules applied in the file `src/configuration/keymatch_model.conf`.

[You can find the documentation here](https://casbin.org/docs/syntax-for-models/)

## Slidev - Setup

### Settings

The underlying technology used to generate courses is [Slidev](https://github.com/slidevjs/slidev). It is not expected to use it directly, OCF will do it for you.

### Themes

Then the most important file is probably the theme. 

It is possible to inherit from them by specifying an `extends.json` file in the theme folder that contains reference to the parent theme such as:

```json
{
    "theme": "parent_theme"
}
```

The theme should be in a separate git repository. [Example](https://usine.solution-libre.fr/open-course-factory/ocf-slidev-theme-sdv) ⚠ not public yet !

## Global Pre-requisites

If you work with docker and dev containers, you can skip this part and just use the dev container.

The golang program needs external libraries to work, they can be installed with:

```shell
go get golang.org/x/text/transform
go get golang.org/x/text/unicode/norm
```

The API uses Swagger for its documentation.

To be sure everything works fine, be sure that your **GOPATH** variable is setted and exported. Then, be sure its **bin** directory is part of your **$PATH** For example, if you installed Goglang in your user home:

```shell
export GOPATH=$HOME/go
export PATH=$PATH:$GOPATH/bin
```

You need to install it either using the go install mechanism or using the last release available in the [github repo](https://github.com/swaggo/swag/releases):

```shell
go get github.com/swaggo/swag
go install github.com/swaggo/swag/cmd/swag@latest
```

and then:

```shell
swag init --parseDependency --parseInternal
```

## Generate (regenerate) API documentation

To regenerate Swagger documentation if needed:

```shell
swag init --parseDependency --parseInternal
```

## Slidev handling

### Slidev outside OCF

You are not supposed to manipulate slidev directly. However, for debug pupose, it can be interesting to understand the underlying calls to slidev.

When you lauch course generation either with API or CLI, it downloads the course files, downloads the theme files and then generate the website.

The command that exports the course in PDF format is:

```shell
docker run --name slidev_export --rm -dit     -v ${PWD}:/slidev     -p 3031:3030     -e NPM_MIRROR="https://registry.npmmirror.com"     tangramor/slidev:playwright
```

To install playwright inside, you may need to do that: 

```shell
docker exec -i slidev_export npx playwright install
```

To build the SPA (made automatically by OCF) :

```shell
docker exec -i slidev_export npx slidev build
```

Run a container embedding the SPA : 

```shell
docker run -d --name myslides --rm -p 3030:80 -v ${PWD}/dist:/usr/share/nginx/html nginx:alpine
```

### Slidev inside OCF

For generation purpose, we had to create a specific slidev image that embeds ocf source code after downloading course & theme. This image should be generated before installing and using OCF.

```shell
docker build -f Dockerfile.slidev -t ocf_slidev .
```

## (DEPRECATED - MARP) I want to generate the slides manually

### Settings

The underlying technology used to generate courses is [Marp](https://github.com/marp-team/marp). It is not expected to use it directly, OCF will do it for you.

To modify Marp behaviour, an additionnal `engine.js` file has been added. It allows in particular to include other .md files in slides and to hide some slides in courses.

### Themes

Then the most important file is probably the theme. For now, we choosed to make our themes inherit from the 'uncover' Marp theme.

It is possible to inherit from them by specifying an `extends.json` file in the theme folder that contains reference to the parent theme such as:

```json
{
    "theme": "parent_theme"
}
```

## Pre-requisites

The golang program needs external libraries to work, they can be installed with:

```shell
go get golang.org/x/text/transform
go get golang.org/x/text/unicode/norm
```

The API uses Swagger for its documentation.

To be sure everything works fine, be sure that your **GOPATH** variable is setted and exported. Then, be sure its **bin** directory is part of your **$PATH** For example, if you installed Goglang in your user home:

```shell
export GOPATH=$HOME/go
export PATH=$PATH:$GOPATH/bin
```

You need to install it either using the go install mechanism or using the last release available in the [github repo](https://github.com/swaggo/swag/releases):

```shell
go get github.com/swaggo/swag
go install github.com/swaggo/swag/cmd/swag@latest
```

and then:

```shell
swag init --parseDependency --parseInternal
```

To be able to use the custom Marp engine located in this repository, you have to install the following npm packages with the following lines:

```shell
npm install @marp-team/marp-core
npm install markdown-it-include --save
npm install markdown-it-container --save
npm install markdown-it-attrs --save
```

You can install it from inside the container if you use Docker.

To enter in the container:

```shell
docker run -it --rm --entrypoint sh -v $PWD:/home/marp/app/ marpteam/marp-cli
```

## (DEPRECATED - MARP) How to build the slides?

### Main.go

**Deprecated**: the CLI mode of OCF was its original conception. Now it is supposed to be used only as a server when the API is started. It is still available to rapidly test a generation.

The courses are defined in `XXX.json` files where `XXX` is the trigram of the course. The golang program `main.go` is made to create a .md file (in `./dist/theme/`) from one of these `.json` files and a `conf.json` file containing data about the author.

`conf.json` **is not provided**: there is a `conf.json.dist` that must be used as basefile to build the appropriate `conf.json` file.

`main.go` has 3 parameters:

- -c: course trigram
- -t: name of the theme (example : sdv)
- -e: type of the course (example : html / pdf)

Ewamples:

```shell
go run main.go -c git -t sdv -e pdf
go run main.go -c golang -t sdv -e html
```

### Practically

**Deprecated**: The whole file based generation system is currently migrated directly into the database. For now it is still necessary, but this section should be removed not too far in the future.

#### Course index file

To build the courses, the main idea is that you have an "index" file which is the main course file, generated in `dist/theme/` directory.

In this file, you put everything specific to the course:

- First and last slides
- Transition between chapters
- Inclusion of chapters

This allows you to change headers and footers exactly as you need following the courses including the chapters you need.

#### How to add figures

It is quite hard to draw things in markdown. The easiest way to do that is to use a software like Inkscape and export it in SVG. If you need to get a Powerpoint diagram, you can copy paste it in inkscape, change it as you need and then export it as SVG as well.

#### Author slide

It is possible to have a author set of slides in the course. To add a author set you have to add it in the `authors` directory. The file name format is `author_xxx.md` where **xxx** is the trigram of the trainer. Then the only thing needed is to change the configuration file to add the author data. Example conf file is available `./courses/conf/conf.json.dist`.

#### Prelude slide

A global introduction is often needed for a course, the `.preludes/THE_NAME_YOU_WANT.md` pages are intented to do so.

#### Schedule slide

A global schedule presentation is often needed for a course, the `.schedules/THE_NAME_YOU_WANT.md` pages are intented to do so.

### What can be improved?

⚠ There is at least one problem left, every image of every course must be copied in the output directory to be sure everything is present. This would be better to have only the needed ressources for a specific generation. An other advantage of doing so would open the possibility to take `.md` files from an URL witch is not possible for now since the inclusion mechanism is based on [markdown-it-include](https://github.com/camelaissani/markdown-it-include) which only works with local files.

Beside, there are still many things to do to make it fully usable by everybody.
