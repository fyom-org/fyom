# fyom — Frontend Roadmap

## Phase 1: Core Layout & Import Trigger (Current)

Build the foundational UI shell and the primary user action — importing a media library.

- [x] Login flow (JWT auth, form validation)
- [ ] Main layout (header, sidebar, content area)
- [ ] Import view (path input, trigger button)
- [ ] Job status polling component
- [ ] API client for library endpoints

## Phase 2: Media Library Grid

Browse imported movies and shows with poster art.

- [ ] Library list view (grid layout with posters)
- [ ] Detail page for individual media items
- [ ] Show → Episodes hierarchy navigation
- [ ] Search and filter controls

## Phase 3: Media Playback

Stream media files directly in the browser.

- [ ] HTML5 video player page
- [ ] Range-request streaming via `/media/:id/stream`
- [ ] Playback controls (play, pause, seek, fullscreen)
- [ ] Subtitle support (if available)

## Phase 4: Polish & Tauri

Refine the experience and wrap in a desktop shell.

- [ ] Dark mode theme
- [ ] Responsive design (mobile-friendly)
- [ ] Tauri desktop shell integration
- [ ] Settings page (server URL, theme preference)
