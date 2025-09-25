# Project Summary

This project is a cli tool written in go that contacts the zoom api and downloads video files. The cli:

# Featues

- looks at environment variables to read zoom autorization credentials
- authorizes with the zoom api
- gets a listing of video files
- creates a local status file containing the status of the downloads
- resumes downloads based on the status file
- retries and times out downloads on zoom api failures
- creates a download directory structure based on <user account>/<year>/<month>/<day>
- generates a download filename based on meeting <topic> in all lowercase with dashes "-" instead of spaces
- downloads the file name to the appropriate sub directory in MP4 format
- also saves a metadata file with the same download file name but with a .json extension. The file contains all of the recordings meta data in json format
- has a --meta-only command line argument that downloads only the metadata files
- has a --limit command line argument that limits the number of recordings to process
- can save downloads to Box using a --box option
- authorization keys and settings for Box will be read from environment variables
- the zoom-to-box --help command displays what environment variables are needed for Zoom and Box auth

# Technical Details

- Uses golang as the programing language
- Used cobra for cobra-cli arguments and setup
- Uses the "Cloud Recording" zoom api that is listed in Open API spec file of zoom-openapi.json

# Development Methodology

- **Test Driven Development (TDD)**: All features must be implemented using TDD methodology
  - Write tests first to define the expected behavior
  - Write minimal code to make tests pass
  - Refactor code while keeping tests green
- **Interface-Driven Design**: Use Go interfaces extensively to improve testability and maintainability
  - Define interfaces for all major components (HTTP clients, file systems, API clients, etc.)
  - Implement concrete types that satisfy these interfaces
  - Use dependency injection with interfaces to enable easy mocking in tests
  - Keep interfaces small and focused (Interface Segregation Principle)
  - Example: `type ZoomAPIClient interface { ListUserRecordings(...) (..., error) }`
- **Quality Gates**: For each feature implementation:
  - All tests must pass (`go test ./...`)
  - Code must build successfully (`go build .`)
  - Code must pass linting (`go vet ./...` and any configured linters)
  - No feature is considered complete until all quality gates pass

# important-instruction-reminders
Do what has been asked; nothing more, nothing less.
NEVER create files unless they're absolutely necessary for achieving your goal.
ALWAYS prefer editing an existing file to creating a new one.
NEVER proactively create documentation files (*.md) or README files. Only create documentation files if explicitly requested by the User.
