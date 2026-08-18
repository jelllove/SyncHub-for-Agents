# Adaptive Main Window Sizing Design

## Goal

Make the AgentConfigSync main window fully visible at startup across Windows display sizes and DPI settings. Large displays should show the complete dashboard in a comfortable, centered window without maximizing it. Small displays should receive a smaller window that stays inside the usable screen area and lets the page scroll.

## Scope

This change affects only initial main-window sizing and responsive overflow behavior. It does not persist window geometry, synchronize window state, maximize the application, or change tray and single-instance behavior.

## Startup sizing

The application reads the Wails primary screen and uses its DPI-aware `WorkArea`, which excludes reserved desktop space such as the Windows taskbar.

The preferred main-window size is:

- Width: 1200 device-independent pixels
- Height: 850 device-independent pixels

The application reserves a 48-pixel margin on every side. Each starting dimension is the smaller of the preferred dimension and the corresponding work-area dimension minus 96 pixels.

The normal minimum interactive size is 640 by 480. If the calculated starting size is smaller because the work area cannot fit that minimum, the configured Wails minimum for that dimension is lowered to the calculated starting dimension. A minimum-size constraint must never force part of the window outside the work area.

The window starts centered on the primary screen's work area. It remains resizable and starts in the normal state rather than maximized.

## Small-screen layout

The existing frontend breakpoint at 800 pixels continues to collapse multi-column dashboard content. The document and application shell must allow vertical scrolling so settings, resource lists, and actions remain reachable when the available height is less than the full content height.

Content is not scaled down. Text, controls, and accessibility sizing remain unchanged; only the native window dimensions and responsive layout change.

## Architecture

A pure sizing function accepts a work-area width and height and returns:

- initial width
- initial height
- minimum width
- minimum height

`guiApplication.configure` obtains the primary screen, calls the sizing function, and supplies the result, target screen, and `WindowCentered` placement to `application.WebviewWindowOptions`.

Window geometry is intentionally not saved. Every launch recalculates it for the current computer, display arrangement, taskbar, and DPI scale, preventing unsuitable geometry from being transferred between computers.

## Error handling

If Wails does not provide a primary screen or reports a non-positive work area, the application logs the condition and falls back to the existing 1080 by 720 starting size with safe minimum constraints. Display detection failure must not prevent startup.

## Testing

Unit tests cover:

- A large work area selecting 1200 by 850.
- A 1920 by 1080-style work area preserving the preferred size and margins.
- A 1366 by 768-style work area reducing the height while keeping the window fully visible.
- A work area smaller than 640 by 480 reducing both starting and minimum dimensions.
- Invalid or missing work-area values using the fallback dimensions.

Regression validation includes the frontend production build, full Go test suite, `go vet`, Windows packaging, and an installed startup smoke test. The smoke test must continue to prove single-instance handoff and profile preservation.
