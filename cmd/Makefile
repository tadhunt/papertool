all: semver
	go mod tidy
	go build -o papermc

semver:
	./mkversion.bash version-make-golang main semver.go

install: all
	cp papermc ${HOME}/bin
