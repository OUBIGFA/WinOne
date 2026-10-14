use serde_json::json;
use winone_desktop::state::{Destination, Draft, Navigation, NavigationResult, build_navigation};

#[test]
fn saving_keeps_revision_and_unrecognized_settings() {
    let original = json!({"revision":7,"theme":"light","futureModule":{"enabled":true}});
    let mut draft = Draft::new(original.clone()).unwrap();
    draft.set("theme", json!("dark")).unwrap();
    assert!(draft.is_dirty());
    let request = draft.begin_save().unwrap();
    assert_eq!(request["revision"], 7);
    assert_eq!(request["futureModule"], json!({"enabled":true}));
    assert!(draft.set("theme", json!("system")).is_err());
    draft.save_failed();
    assert!(draft.is_dirty());
    assert_eq!(draft.value()["theme"], "dark");
    draft.begin_save().unwrap();
    draft.save_succeeded(json!({"revision":8,"theme":"dark","futureModule":{"enabled":true}})).unwrap();
    assert!(!draft.is_dirty());
    assert_eq!(draft.value()["revision"], 8);
}

#[test]
fn cancelling_unsaved_navigation_keeps_page_and_draft() {
    let mut navigation = Navigation::new("wintray");
    assert_eq!(navigation.request(Destination::Page("capture".into()), true, false), NavigationResult::Confirm);
    navigation.cancel();
    assert_eq!(navigation.page(), "wintray");
    assert_eq!(navigation.pending(), None);
    assert_eq!(navigation.request(Destination::Close, true, true), NavigationResult::Busy);
    assert_eq!(navigation.discard(false), None);
}

#[test]
fn module_navigation_uses_host_catalog_and_preserves_builtin_order() {
    let catalog = json!([
        {"id":"publisher.tools","name":{"zh-CN":"扩展工具","en-US":"Tools"}},
        {"id":"wintray","name":{"zh-CN":"程序管理","en-US":"Program management"}},
        {"id":"capture","name":{"zh-CN":"截图","en-US":"Capture"}},
        {"id":"always-on-top","name":{"zh-CN":"置顶功能","en-US":"Always on Top"}}
    ]);
    let pages = build_navigation(&catalog, "zh-CN").unwrap();
    assert_eq!(pages.iter().map(|p| p.id.as_str()).collect::<Vec<_>>(),
        ["overview", "capture", "always-on-top", "wintray", "publisher.tools", "settings"]);
    assert_eq!(pages[4].title, "扩展工具");
    let mut invalid = catalog;
    invalid.as_array_mut().unwrap().push(json!({"id":"settings","name":{"zh-CN":"伪设置","en-US":"Fake"}}));
    assert!(build_navigation(&invalid, "zh-CN").is_err());
}
