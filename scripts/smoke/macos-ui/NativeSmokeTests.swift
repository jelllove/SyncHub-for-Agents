import XCTest

final class NativeSmokeTests: XCTestCase {
    private var app: XCUIApplication!
    private var evidence: URL!
    private var profile: URL!

    override func setUpWithError() throws {
        continueAfterFailure = false
        let environment = ProcessInfo.processInfo.environment
        evidence = URL(fileURLWithPath: try XCTUnwrap(environment["SMOKE_EVIDENCE_DIR"]))
        profile = evidence.appendingPathComponent("profile")
        let installed = try XCTUnwrap(environment["SMOKE_APP_PATH"])
        XCTAssertTrue(FileManager.default.fileExists(atPath: installed + "/Contents/MacOS/SyncHub"))
        app = XCUIApplication(bundleIdentifier: "io.github.qinqingxu.synchub")
        app.launchEnvironment = [
            "HOME": profile.path,
            "GIT_CONFIG_GLOBAL": "/dev/null",
            "GIT_CONFIG_NOSYSTEM": "1",
            "SSH_AUTH_SOCK": "",
        ]
        app.launch()
        XCTAssertTrue(app.wait(for: .runningForeground, timeout: 30))
    }

    override func tearDownWithError() throws {
        if let app = app {
            let capture = XCTAttachment(screenshot: app.screenshot())
            capture.name = "final-native-window"
            capture.lifetime = .keepAlways
            add(capture)
            app.terminate()
        }
        try super.tearDownWithError()
    }

    private func capture(_ name: String) throws {
        let screenshot = app.screenshot()
        try screenshot.pngRepresentation.write(to: evidence.appendingPathComponent("screenshots/\(name).png"))
        let attachment = XCTAttachment(screenshot: screenshot)
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
    }

    func testInstalledApplicationOnboardingAndValidation() throws {
        let start = app.buttons["Get started"]
        XCTAssertTrue(start.waitForExistence(timeout: 30), app.debugDescription)
        try capture("01-welcome")
        start.click()
        let field = app.textFields.firstMatch
        XCTAssertTrue(field.waitForExistence(timeout: 10), app.debugDescription)
        XCTAssertTrue(app.staticTexts["Connect your private repository"].exists)
        try capture("02-repository")
        field.click()
        field.typeText("not-a-valid-repository")
        app.buttons["Continue"].click()
        let error = app.staticTexts["repository URL must contain owner and repository"]
        XCTAssertTrue(error.waitForExistence(timeout: 10), app.debugDescription)
        XCTAssertTrue(field.exists, "Validation failure must keep repository input available")
        try capture("03-validation")
        XCTAssertTrue(FileManager.default.fileExists(atPath: profile.appendingPathComponent(".synchub").path),
                      "Application must use the isolated HOME")
        XCTAssertFalse(FileManager.default.fileExists(atPath: profile.appendingPathComponent(".synchub/config.yaml").path),
                       "Invalid input must not complete onboarding or enable real synchronization")
    }
}
