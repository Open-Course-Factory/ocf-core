<!-- markdownlint-disable MD013 MD024 -->

# OCF Core

This is the core part of Open Course Factory. It is mainly an API which provides the ability to store courses and generate them in HTML or PDF.

This product is developped within [Solution Libre](https://www.solution-libre.fr/).

⚠ Currently under development, many things can still change, expect bugs and problems.

## How the slides work?

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

## How to build the slides?

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

## API Documentation - WIP

This part is under active develoment and is expected to be fully operationnal for september (2023 !)

### Database

The expected database is PostgreSQL. To help the development, a fallback database based on SQLite should allow to make it work without a proper database setup.

### Start the server

Without arguments, the program starts a web server which is mainly an API.

```shell
go run main.go
[GIN-debug] [WARNING] Creating an Engine instance with the Logger and Recovery middleware already attached.

[GIN-debug] [WARNING] Running in "debug" mode. Switch to "release" mode in production.
 - using env:   export GIN_MODE=release
 - using code:  gin.SetMode(gin.ReleaseMode)

[GIN-debug] POST   /api/v1/users/            --> soli/formations/src/auth/routes/userRoutes.UserController.AddUser-fm (4 handlers)
[GIN-debug] GET    /api/v1/users/            --> soli/formations/src/auth/routes/userRoutes.UserController.GetUsers-fm (5 handlers)
[GIN-debug] GET    /api/v1/users/:id         --> soli/formations/src/auth/routes/userRoutes.UserController.GetUser-fm (5 handlers)
[GIN-debug] DELETE /api/v1/users/:id         --> soli/formations/src/auth/routes/userRoutes.UserController.DeleteUser-fm (5 handlers)
[GIN-debug] PATCH  /api/v1/users/            --> soli/formations/src/auth/routes/userRoutes.UserController.EditUserSelf-fm (5 handlers)
[GIN-debug] PUT    /api/v1/users/:id         --> soli/formations/src/auth/routes/userRoutes.UserController.EditUser-fm (5 handlers)
[GIN-debug] POST   /api/v1/roles/            --> soli/formations/src/auth/routes/roleRoutes.RoleController.AddRole-fm (5 handlers)
[GIN-debug] GET    /api/v1/roles/            --> soli/formations/src/auth/routes/roleRoutes.RoleController.GetRoles-fm (5 handlers)
[GIN-debug] GET    /api/v1/roles/:id         --> soli/formations/src/auth/routes/roleRoutes.RoleController.GetRole-fm (5 handlers)
[GIN-debug] DELETE /api/v1/roles/:id         --> soli/formations/src/auth/routes/roleRoutes.RoleController.DeleteRole-fm (5 handlers)
[GIN-debug] PUT    /api/v1/roles/:id         --> soli/formations/src/auth/routes/roleRoutes.RoleController.EditRole-fm (5 handlers)
[GIN-debug] POST   /api/v1/groups/           --> soli/formations/src/auth/routes/groupRoutes.GroupController.AddGroup-fm (5 handlers)
[GIN-debug] GET    /api/v1/groups/           --> soli/formations/src/auth/routes/groupRoutes.GroupController.GetGroups-fm (5 handlers)
[GIN-debug] GET    /api/v1/groups/:id        --> soli/formations/src/auth/routes/groupRoutes.GroupController.GetGroup-fm (5 handlers)
[GIN-debug] DELETE /api/v1/groups/:id        --> soli/formations/src/auth/routes/groupRoutes.GroupController.DeleteGroup-fm (5 handlers)
[GIN-debug] PUT    /api/v1/groups/:id        --> soli/formations/src/auth/routes/groupRoutes.GroupController.EditGroup-fm (5 handlers)
[GIN-debug] POST   /api/v1/login/            --> soli/formations/src/auth/routes/loginRoutes.LoginController.Login-fm (4 handlers)
[GIN-debug] POST   /api/v1/refresh/          --> soli/formations/src/auth/routes/loginRoutes.LoginController.RefreshToken-fm (4 handlers)
[GIN-debug] POST   /api/v1/courses/generate  --> soli/formations/src/courses/routes/courseRoutes.CourseController.GenerateCourse-fm (4 handlers)
[GIN-debug] POST   /api/v1/courses/create    --> soli/formations/src/courses/routes/courseRoutes.CourseController.AddCourse-fm (4 handlers)
[GIN-debug] GET    /swagger/*any             --> github.com/swaggo/gin-swagger.CustomWrapHandler.func1 (4 handlers)
[GIN-debug] [WARNING] You trusted all proxies, this is NOT safe. We recommend you to set a value.
Please check https://pkg.go.dev/github.com/gin-gonic/gin#readme-don-t-trust-all-proxies for details.
[GIN-debug] Listening and serving HTTP on :8000
```

### Swagger

With the default debug configuration, a Swagger documentation is available at:

```shell
http://localhost:8000/swagger/index.html
```

### Permissions

Permission system is something we currently work on. It is not finished yet. For now it is possible to do everything once a user a loggued, this is not ready for production.
