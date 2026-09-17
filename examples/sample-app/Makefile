.PHONY: all app dmg pkg clean run

all: app dmg pkg

app:
	@./build.sh

run: app
	@open dist/SampleApp.app

clean:
	@rm -rf dist
