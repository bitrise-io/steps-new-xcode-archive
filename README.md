# Xcode Archive & Export for iOS

[![Step changelog](https://shields.io/github/v/release/bitrise-steplib/steps-xcode-archive?include_prereleases&label=changelog&color=blueviolet)](https://github.com/bitrise-steplib/steps-xcode-archive/releases)

Automatically manages your code signing assets, archives and exports an .ipa in one Step.

<details>
<summary>Description</summary>

The Step archives your Xcode project by running the `xcodebuild archive` command and then exports the archive into an .ipa file with the `xcodebuild -exportArchive` command.
This .ipa file can be shared, installed on test devices, or uploaded to the App Store Connect.
With this Step, you can use automatic code signing in a [CI environment without having to use Xcode](https://developer.apple.com/documentation/xcode-release-notes/xcode-13-release-notes).
In short, the Step:
- Logs you into your Apple Developer account based on the [Apple service connection you provide on Bitrise](https://devcenter.bitrise.io/en/accounts/connecting-to-services/apple-services-connection.html).
- Downloads any provisioning profiles needed for your project based on the **Distribution method**.
- Runs your build. It archives your Xcode project by running the `xcodebuild archive` command and exports the archive into an .ipa file with the `xcodebuild -exportArchive` command.
This .ipa file can be shared and installed on test devices, or uploaded to App Store Connect.

### Configuring the Step
Before you start:
- Make sure you have connected your [Apple Service account to Bitrise](https://devcenter.bitrise.io/en/accounts/connecting-to-services/apple-services-connection.html).
Alternatively, you can upload certificates and profiles to Bitrise manually, then use the Certificate and Profile installer step before Xcode Archive
- Make sure certificates are uploaded to Bitrise's **Code Signing** tab. The right provisioning profiles are automatically downloaded from Apple as part of the automatic code signing process.

To configure the Step:
1. **Project path**: Add the path where the Xcode Project or Workspace is located.
2. **Scheme**: Add the scheme name you wish to archive your project later.
3. **Distribution method**: Select the method Xcode should sign your project: development, app-store, ad-hoc, or enterprise.

Under **xcodebuild configuration**:
1. **Build configuration**: Specify Xcode Build Configuration. The Step uses the provided Build Configuration's Build Settings to understand your project's code signing configuration. If not provided, the Archive action's default Build Configuration will be used.
2. **Build settings (xcconfig)**: Build settings to override the project's build settings. Can be the contents, file path or empty.
3. **Perform clean action**: If this input is set, a `clean` xcodebuild action will be performed besides the `archive` action.

Under **Xcode build log formatting**:
1. **Log formatter**: Defines how `xcodebuild` command's log is formatted. Available options are `xcpretty`: The xcodebuild command's output will be prettified by xcpretty. `xcodebuild`: Only the last 20 lines of raw xcodebuild output will be visible in the build log.
The raw xcodebuild log is exported in both cases.

Under **Automatic code signing**:
1. **Automatic code signing method**: Select the Apple service connection you want to use for code signing. Available options: `off` if you don't do automatic code signing, `api-key` [if you use API key authorization](https://devcenter.bitrise.io/en/accounts/connecting-to-services/connecting-to-an-apple-service-with-api-key.html), and `apple-id` [if you use Apple ID authorization](https://devcenter.bitrise.io/en/accounts/connecting-to-services/connecting-to-an-apple-service-with-apple-id.html).
2. **Register test devices on the Apple Developer Portal**: If this input is set, the Step will register the known test devices on Bitrise from team members with the Apple Developer Portal. Note that setting this to `yes` may cause devices to be registered against your limited quantity of test devices in the Apple Developer Portal, which can only be removed once annually during your renewal window.
3. **The minimum days the Provisioning Profile should be valid**: If this input is set to >0, the managed Provisioning Profile will be renewed if it expires within the configured number of days. Otherwise the Step renews the managed Provisioning Profile if it is expired.
4. The **Code signing certificate URL**, the **Code signing certificate passphrase**, the **Keychain path**, and the **Keychain password** inputs are automatically populated if certificates are uploaded to Bitrise's **Code Signing** tab. If you store your files in a private repo, you can manually edit these fields.

If you want to set the Apple service connection credentials on the step-level (instead of using the one configured in the App Settings), use the Step inputs in the **App Store Connect connection override** category. Note that this only works if **Automatic code signing method** is set to `api-key`.

Under **IPA export configuration**:
1. **Developer Portal team**: Add the Developer Portal team's name to use for this export. This input defaults to the team used to build the archive.
2. **Rebuild from bitcode**: For non-App Store exports, should Xcode re-compile the app from bitcode?
3. **Include bitcode**: For App Store exports, should the package include bitcode?
4. **iCloud container environment**: If the app is using CloudKit, this input configures the `com.apple.developer.icloud-container-environment` entitlement. Available options vary depending on the type of provisioning profile used, but may include: `Development` and `Production`.
5. **Export options plist content**: Specifies a `plist` file content that configures archive exporting. If not specified, the Step will auto-generate it.

Under **Step Output Export configuration**:
1. **Output directory path**: This directory will contain the generated artifacts.
2. **Export all dSYMs**: Export additional dSYM files besides the app dSYM file for Frameworks.
3. **Override generated artifact names**:  This name is used as basename for the generated Xcode archive, app, `.ipa` and dSYM files. If not specified, the Product Name (`PRODUCT_NAME`) Build settings value will be used. If Product Name is not specified, the Scheme will be used.

Under **Caching**:
1. **Enable collecting cache content**: Defines what cache content should be automatically collected. Available options are `none`: Disable collecting cache content and `swift_packages`: Collect Swift PM packages added to the Xcode project

Under Debugging:
1. **Verbose logging***: You can set this input to `yes` to produce more informative logs.
</details>

## 🧩 Get started

Add this step directly to your workflow in the [Bitrise Workflow Editor](https://devcenter.bitrise.io/steps-and-workflows/steps-and-workflows-index/).

You can also run this step directly with [Bitrise CLI](https://github.com/bitrise-io/bitrise).

### Examples

Build a development IPA:
```yaml
- xcode-archive:
    inputs:
    - project_path: ./ios-sample/ios-sample.xcodeproj
    - scheme: ios-sample
    - distribution_method: development
```

Build a development IPA with custom xcconfig content:
```yaml
- xcode-archive:
    inputs:
    - project_path: ./ios-sample/ios-sample.xcodeproj
    - scheme: ios-sample
    - distribution_method: development
    - xcconfig_content: |
        CODE_SIGN_IDENTITY = Apple Development
```

Build a development IPA with overridden code signing setting:
```yaml
- xcode-archive:
    inputs:
    - project_path: ./ios-sample/ios-sample.xcodeproj
    - scheme: ios-sample
    - distribution_method: development
    - xcconfig_content: |
        DEVELOPMENT_TEAM = XXXXXXXXXX
        CODE_SIGN_IDENTITY = Apple Development
        PROVISIONING_PROFILE_SPECIFIER = 12345678-90ab-cdef-0123-4567890abcde
```

Build a development IPA with custom xcconfig file path:
```yaml
- xcode-archive:
    inputs:
    - project_path: ./ios-sample/ios-sample.xcodeproj
    - scheme: ios-sample
    - distribution_method: development
    - xcconfig_content: ./ios-sample/ios-sample/Configurations/Dev.xcconfig
```

## ⚙️ Configuration

<details>
<summary>Inputs</summary>

| Key | Description | Flags | Default |
| --- | --- | --- | --- |
| `project_path` | Xcode Project (`.xcodeproj`) or Workspace (`.xcworkspace`) path.  The input value sets xcodebuild's `-project` or `-workspace` option. | required | `$BITRISE_PROJECT_PATH` |
| `scheme` | Xcode Scheme name.  The input value sets xcodebuild's `-scheme` option. | required | `$BITRISE_SCHEME` |
| `distribution_method` | Describes how Xcode should export the archive. | required | `development` |
| `testflight_internal_testing_only` | Sets if the archive is for testflight internal testing. `distribution_method` should be app-store. |  | `no` |
| `configuration` | Xcode Build Configuration.  If not specified, the default Build Configuration will be used.  The input value sets xcodebuild's `-configuration` option. |  |  |
| `xcconfig_content` | Build settings to override the project's build settings, using xcodebuild's `-xcconfig` option.  You can't define `-xcconfig` option in `Additional options for the xcodebuild command` if this input is set.  If empty, no setting is changed. When set it can be either: 1.  Existing `.xcconfig` file path.      Example:      `./ios-sample/ios-sample/Configurations/Dev.xcconfig`  2.  The contents of a newly created temporary `.xcconfig` file. (This is the default.)      Build settings must be separated by newline character (`\n`).      Example:     ```     COMPILER_INDEX_STORE_ENABLE = NO     ONLY_ACTIVE_ARCH[config=Debug][sdk=*][arch=*] = YES     ``` |  | `COMPILER_INDEX_STORE_ENABLE = NO` |
| `perform_clean_action` | If this input is set, `clean` xcodebuild action will be performed besides the `archive` action. | required | `no` |
| `xcodebuild_options` | Additional options to be added to the executed xcodebuild command.  Prefer using `Build settings (xcconfig)` input for specifying `-xcconfig` option. You can't use both.  `-destination` is set automatically, unless specified explicitely. |  |  |
| `log_formatter` | Defines how `xcodebuild` command's log is formatted.  Available options:  - `xcpretty`: The xcodebuild command's output will be prettified by xcpretty. - `xcodebuild`: Only the last 20 lines of raw xcodebuild output will be visible in the build log.  The raw xcodebuild log will be exported in both cases. | required | `xcpretty` |
| `automatic_code_signing` | This input determines which Bitrise Apple service connection should be used for automatic code signing.  Available values: - `off`: Do not do any auto code signing. - `api-key`: [Bitrise Apple Service connection with API Key](https://devcenter.bitrise.io/getting-started/connecting-to-services/setting-up-connection-to-an-apple-service-with-api-key/). - `apple-id`: [Bitrise Apple Service connection with Apple ID](https://devcenter.bitrise.io/getting-started/connecting-to-services/connecting-to-an-apple-service-with-apple-id/). | required | `off` |
| `register_test_devices` | If this input is set, the Step will register the known test devices on Bitrise from team members with the Apple Developer Portal.  Note that setting this to yes may cause devices to be registered against your limited quantity of test devices in the Apple Developer Portal, which can only be removed once annually during your renewal window. | required | `no` |
| `test_device_list_path` | If this input is set, the Step will register the listed devices from this file with the Apple Developer Portal.  The format of the file is a comma separated list of the identifiers. For example: `00000000–0000000000000001,00000000–0000000000000002,00000000–0000000000000003`  And in the above example the registered devices appear with the name of `Device 1`, `Device 2` and `Device 3` in the Apple Developer Portal.  Note that setting this will have a higher priority than the Bitrise provided devices list. |  |  |
| `min_profile_validity` | If this input is set to >0, the managed Provisioning Profile will be renewed if it expires within the configured number of days.  Otherwise the Step renews the managed Provisioning Profile if it is expired. | required | `0` |
| `certificate_url_list` | URL of the code signing certificate to download.  Multiple URLs can be specified, separated by a pipe (`\|`) character.  Local file path can be specified, using the `file://` URL scheme. | required, sensitive | `$BITRISE_CERTIFICATE_URL` |
| `passphrase_list` | Passphrases for the provided code signing certificates.  Specify as many passphrases as many Code signing certificate URL provided, separated by a pipe (`\|`) character.  Certificates without a passphrase: for using a single certificate, leave this step input empty. For multiple certificates, use the separator as if there was a passphrase (examples: `pass\|`, `\|pass\|`, `\|`) | sensitive | `$BITRISE_CERTIFICATE_PASSPHRASE` |
| `keychain_path` | Path to the Keychain where the code signing certificates will be installed. | required | `$HOME/Library/Keychains/login.keychain` |
| `keychain_password` | Password for the provided Keychain. | required, sensitive | `$BITRISE_KEYCHAIN_PASSWORD` |
| `fallback_provisioning_profile_url_list` | If set, provided provisioning profiles will be used on Automatic code signing error.  URL of the provisioning profile to download. Multiple URLs can be specified, separated by a newline or pipe (`\|`) character.  You can specify a local path as well, using the `file://` scheme. For example: `file://./BuildAnything.mobileprovision`.  Can also provide a local directory that contains files with `.mobileprovision` extension. For example: `./profilesDirectory/`  | sensitive |  |
| `export_development_team` | The Developer Portal team to use for this export  Defaults to the team used to build the archive.  Defining this is also required when Automatic Code Signing is set to `apple-id` and the connected account belongs to multiple teams. |  |  |
| `compile_bitcode` | For __non-App Store__ exports, should Xcode re-compile the app from bitcode? | required | `yes` |
| `upload_bitcode` | For __App Store__ exports, should the package include bitcode? | required | `yes` |
| `icloud_container_environment` | If the app is using CloudKit, this configures the `com.apple.developer.icloud-container-environment` entitlement.  Available options vary depending on the type of provisioning profile used, but may include: `Development` and `Production`. |  |  |
| `export_options_plist_content` | Specifies a plist file content that configures archive exporting.  If not specified, the Step will auto-generate it. |  |  |
| `output_dir` | This directory will contain the generated artifacts. | required | `$BITRISE_DEPLOY_DIR` |
| `export_all_dsyms` | Export additional dSYM files besides the app dSYM file for Frameworks. | required | `yes` |
| `artifact_name` | This name will be used as basename for the generated Xcode Archive, App, IPA and dSYM files.  If not specified, the Product Name (`PRODUCT_NAME`) Build settings value will be used. If Product Name is not specified, the Scheme will be used. |  |  |
| `cache_level` | Defines what cache content should be automatically collected.  Available options:  - `none`: Disable collecting cache content - `swift_packages`: Collect Swift PM packages added to the Xcode project | required | `swift_packages` |
| `api_key_path` | Local path or remote URL to the private key (p8 file) for App Store Connect API. This overrides the Bitrise-managed API connection, only set this input if you want to control the API connection on a step-level. Most of the time it's easier to set up the connection on the App Settings page on Bitrise. The input value can be a file path (eg. `$TMPDIR/private_key.p8`) or an HTTPS URL. This input only takes effect if the other two connection override inputs are set too (`api_key_id`, `api_key_issuer_id`). |  |  |
| `api_key_id` | Private key ID used for App Store Connect authentication. This overrides the Bitrise-managed API connection, only set this input if you want to control the API connection on a step-level. Most of the time it's easier to set up the connection on the App Settings page on Bitrise. This input only takes effect if the other two connection override inputs are set too (`api_key_path`, `api_key_issuer_id`). |  |  |
| `api_key_issuer_id` | Private key issuer ID used for App Store Connect authentication. This overrides the Bitrise-managed API connection, only set this input if you want to control the API connection on a step-level. Most of the time it's easier to set up the connection on the App Settings page on Bitrise. This input only takes effect if the other two connection override inputs are set too (`api_key_path`, `api_key_id`). |  |  |
| `verbose_log` | If this input is set, the Step will print additional logs for debugging. | required | `no` |
</details>

<details>
<summary>Outputs</summary>

| Environment Variable | Description |
| --- | --- |
| `BITRISE_IPA_PATH` | Local path of the created .ipa file |
| `BITRISE_APP_DIR_PATH` | Local path of the generated `.app` directory |
| `BITRISE_DSYM_DIR_PATH` | This Environment Variable points to the path of the directory which contains the dSYMs files. If `export_all_dsyms` is set to `yes`, the Step will collect every dSYM (app dSYMs and framwork dSYMs). |
| `BITRISE_DSYM_PATH` | This Environment Variable points to the path of the zip file which contains the dSYM files. If `export_all_dsyms` is set to `yes`, the Step will also collect framework dSYMs in addition to app dSYMs. |
| `BITRISE_XCARCHIVE_PATH` | The created .xcarchive file's path |
| `BITRISE_XCARCHIVE_ZIP_PATH` | The created .xcarchive.zip file's path. |
| `BITRISE_XCODEBUILD_ARCHIVE_LOG_PATH` | The file path of the raw `xcodebuild archive` command log. The log is placed into the `Output directory path`. |
| `BITRISE_XCODEBUILD_EXPORT_ARCHIVE_LOG_PATH` | The file path of the raw `xcodebuild -exportArchive` command log. The log is placed into the `Output directory path`. |
| `BITRISE_IDEDISTRIBUTION_LOGS_PATH` | Exported when `xcodebuild -exportArchive` command fails. |
</details>

## 🙋 Contributing

We welcome [pull requests](https://github.com/bitrise-steplib/steps-xcode-archive/pulls) and [issues](https://github.com/bitrise-steplib/steps-xcode-archive/issues) against this repository.

For pull requests, work on your changes in a forked repository and use the Bitrise CLI to [run step tests locally](https://devcenter.bitrise.io/bitrise-cli/run-your-first-build/).

**Note:** this step's end-to-end tests (defined in `e2e/bitrise.yml`) are working with secrets which are intentionally not stored in this repo. External contributors won't be able to run those tests. Don't worry, if you open a PR with your contribution, we will help with running tests and make sure that they pass.

Learn more about developing steps:

- [Create your own step](https://devcenter.bitrise.io/contributors/create-your-own-step/)
- [Testing your Step](https://devcenter.bitrise.io/contributors/testing-and-versioning-your-steps/)
