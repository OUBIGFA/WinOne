use serde_json::json;
use winone_desktop::programs::{field, updated};

#[test]
fn add_and_replace_keep_defaults_identity_and_schedule() {
    let mut program = winone_desktop::programs::created("stable",r"C:\Apps\Editor.exe").unwrap();
    assert_eq!(program["name"],"Editor");
    assert_eq!(program["runOnStartup"],true);
    assert_eq!(program["trayBehavior"]["autoMinimizeAndHideOnLaunch"],true);
    program["args"] = json!("--keep");
    program["schedule"]["enabled"] = json!(true);
    program["collectTrayIcon"] = json!(true);
    let replaced = winone_desktop::programs::replaced(&program,r"C:\Scripts\job.ps1").unwrap();
    assert_eq!(replaced["id"],"stable");
    assert_eq!(replaced["args"],"--keep");
    assert_eq!(replaced["schedule"],program["schedule"]);
    assert_eq!(replaced["collectTrayIcon"],false);
    assert_eq!(replaced["launchHiddenInBackground"],true);
    assert_eq!(replaced["launchViaLogonTask"],false);
    assert_eq!(replaced["trayBehavior"]["autoMinimizeAndHideOnLaunch"],false);
    assert!(winone_desktop::programs::created("stable","").is_err());
}

#[test]
fn reorder_moves_only_the_selected_program_and_rejects_missing_targets() {
    let settings = json!({"managedApps":[{"id":"a","args":"keep-a"},{"id":"b","args":"keep-b"},{"id":"c","args":"keep-c"}]});
    let reordered = winone_desktop::programs::reordered(&settings,"a",2).unwrap();
    assert_eq!(reordered,json!([{"id":"b","args":"keep-b"},{"id":"c","args":"keep-c"},{"id":"a","args":"keep-a"}]));
    assert!(winone_desktop::programs::reordered(&settings,"missing",0).is_err());
    assert!(winone_desktop::programs::reordered(&settings,"a",3).is_err());
    assert_eq!(settings["managedApps"][0]["id"],"a");
}

#[test]
fn edits_follow_id_and_keep_other_programs_and_unknown_fields() {
    let settings = json!({"managedApps":[
        {"id":"second","name":"Second","schedule":{"enabled":false},"future":{"keep":1}},
        {"id":"first","name":"First","schedule":{"enabled":false}}]});
    let programs = updated(&settings, "first", "schedule.enabled", json!(true)).unwrap();
    assert_eq!(programs[0], settings["managedApps"][0]);
    assert_eq!(programs[1]["schedule"]["enabled"], true);
    assert!(updated(&settings, "removed", "name", json!("Wrong")).is_err());
    assert!(updated(&settings, "first", "id", json!("Wrong")).is_err());
}

#[test]
fn startup_modes_are_exclusive_and_scripts_keep_their_constraint() {
    let mut settings = json!({"managedApps":[{"id":"one","exePath":"app.exe","launchViaLogonTask":false,
        "launchHiddenInBackground":false,"trayBehavior":{"autoMinimizeAndHideOnLaunch":false,"closeDelaySeconds":9}}]});
    for mode in ["task","tray","hidden","normal"] {
        settings["managedApps"] = updated(&settings, "one", "mode", json!(mode)).unwrap();
        assert_eq!(field(&settings,"one","mode"), mode);
        assert_eq!(settings["managedApps"][0]["trayBehavior"]["closeDelaySeconds"],9);
    }
    settings["managedApps"][0]["exePath"] = json!("script.ps1");
    assert!(updated(&settings,"one","mode",json!("task")).is_err());
    assert!(updated(&settings,"one","mode",json!("hidden")).is_ok());
}
