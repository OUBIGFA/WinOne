use mermaid_rs_renderer::{Config, Layout, LayoutConfig, Theme};
use resvg::{tiny_skia, usvg};
use std::path::PathBuf;
use std::sync::Arc;
mod flowchart;

/// One bounded, local Mermaid render. SVG external-image resolution is disabled;
/// only installed Windows fonts are loaded, never paths provided in diagram text.
pub fn render_diagram(source: &str, dark: bool) -> Result<Vec<u8>, String> {
    let (layout, theme, config) = diagram_layout(source, dark)?;
    let svg = mermaid_rs_renderer::render_svg(&layout, &theme, &config);
    rasterize(&svg)
}

// Keep parse/layout separate from rasterization so the final geometry can be
// checked before it becomes pixels. Init directives retain upstream semantics.
fn diagram_layout(source: &str, dark: bool) -> Result<(Layout, Theme, LayoutConfig), String> {
    if source.len() > 65_536 || source.lines().count() > 1500 {
        return Err("Mermaid diagram is too large".into());
    }
    let parsed = mermaid_rs_renderer::parse_mermaid_strict(source)
        .map_err(|_| "Mermaid syntax is invalid or unsupported".to_string())?;
    if parsed.graph.nodes.len() > 256 || parsed.graph.edges.len() > 2048 {
        return Err("Split this large Mermaid diagram into smaller diagrams".into());
    }
    let mut theme = if dark { Theme::dark() } else { Theme::modern() };
    theme.font_family = "Segoe UI, Microsoft YaHei, sans-serif".into();
    let mut config = Config { theme, ..Default::default() };
    if let Some(init) = parsed.init_config {
        config = mermaid_rs_renderer::merge_init_config(config, init);
    }
    let layout = flowchart::layout(&parsed.graph, &config.theme, &config.layout);
    Ok((layout, config.theme, config.layout))
}

fn rasterize(svg: &str) -> Result<Vec<u8>, String> {
    if svg.len() > 8 * 1024 * 1024 { return Err("Rendered diagram is too large".into()); }
    let mut fonts = usvg::fontdb::Database::new();
    let system = std::env::var_os("SystemRoot").ok_or("Windows font directory is unavailable")?;
    let directory = PathBuf::from(system).join("Fonts");
    for name in ["segoeui.ttf", "segoeuib.ttf", "msyh.ttc", "msyhbd.ttc", "consola.ttf"] {
        let path = directory.join(name);
        if path.is_file() { fonts.load_font_file(path).map_err(|_| "System font could not be loaded")?; }
    }
    if fonts.faces().next().is_none() { return Err("No system font is available".into()); }
    fonts.set_sans_serif_family("Segoe UI");
    let options = usvg::Options {
        font_family: "Segoe UI".into(),
        fontdb: Arc::new(fonts),
        image_href_resolver: usvg::ImageHrefResolver {
            resolve_data: Box::new(|_, _, _| None),
            resolve_string: Box::new(|_, _| None),
        },
        ..Default::default()
    };
    let tree = usvg::Tree::from_str(&svg, &options).map_err(|_| "Diagram SVG could not be read")?;
    let size = tree.size();
    let scale = 2.0_f32.min(2400.0 / size.width()).min(4000.0 / size.height())
        .min((4_000_000.0 / (size.width() * size.height())).sqrt());
    if !scale.is_finite() || scale <= 0.0 { return Err("Diagram size is invalid".into()); }
    let mut pixels = tiny_skia::Pixmap::new((size.width()*scale).ceil() as u32, (size.height()*scale).ceil() as u32)
        .ok_or("Diagram allocation failed")?;
    resvg::render(&tree, tiny_skia::Transform::from_scale(scale, scale), &mut pixels.as_mut());
    pixels.encode_png().map_err(|_| "Diagram PNG could not be encoded".into())
}

#[cfg(test)]
mod tests {
    use super::*;

    // A node or another label must not cover any part of a connector caption.
    // This catches the upstream final own-edge nudge moving text into a node.
    #[test]
    fn long_edge_labels_remain_readable_in_all_directions() {
        let fixture = include_str!("../tests/fixtures/long-edge-labels.mmd");
        for direction in ["TD", "LR", "RL", "BT"] {
            let source = fixture.replacen("flowchart TD", &format!("flowchart {direction}"), 1);
            let (layout, _, _) = diagram_layout(&source, false).unwrap();
            let mut labels = Vec::new();
            for edge in &layout.edges {
                let (Some(label), Some((x, y))) = (&edge.label, edge.label_anchor) else { continue };
                let rect = (x-label.width/2.0, y-label.height/2.0, x+label.width/2.0, y+label.height/2.0);
                let overlaps = |other: (f32, f32, f32, f32)| {
                    rect.0 < other.2 && rect.2 > other.0 && rect.1 < other.3 && rect.3 > other.1
                };
                for node in layout.nodes.values().filter(|node| !node.hidden) {
                    assert!(!overlaps((node.x, node.y, node.x+node.width, node.y+node.height)),
                        "{direction}: {} -> {} label {:?} intersects node {}", edge.from, edge.to, rect, node.id);
                }
                assert!(!labels.iter().copied().any(overlaps), "{direction}: overlapping edge captions");
                assert!(rect.0 >= 0.0 && rect.1 >= 0.0 && rect.2 <= layout.width && rect.3 <= layout.height,
                    "{direction}: caption clipped by canvas");
                labels.push(rect);
            }
            assert_eq!(labels.len(), 6, "all original captions must remain visible");
            assert_eq!(layout.nodes.len(), 6, "captions must not become process nodes");
            assert_eq!(layout.edges.len(), 6, "one logical connector per source edge");
            let quality = mermaid_rs_renderer::layout::flowchart_quality_metrics(&layout).unwrap();
            assert_eq!(quality.hard_violation_count(), 0, "routes and arrow endpoints remain valid");
        }
    }

    #[test]
    fn caption_layout_preserves_arrow_meaning_style_and_self_loops() {
        let source = "flowchart LR\n A <-->|双向通信| B\n A -.->|备用连接| B\n B -->|重新尝试| B\n __winone_caption_0[真实节点]\n linkStyle 1 stroke:#ff0000,stroke-width:3px";
        let (layout, _, _) = diagram_layout(source, false).unwrap();
        assert_eq!(layout.nodes.len(), 3);
        assert_eq!(layout.nodes["__winone_caption_0"].label.lines, ["真实节点"]);
        assert_eq!(layout.edges.len(), 3);
        for (index, (from, to, text)) in [("A", "B", "双向通信"), ("A", "B", "备用连接"), ("B", "B", "重新尝试")].into_iter().enumerate() {
            let edge = &layout.edges[index];
            assert_eq!((&*edge.from, &*edge.to), (from, to));
            assert_eq!(edge.label.as_ref().unwrap().lines, [text]);
            assert!(edge.arrow_end);
            assert_eq!(edge.arrow_start, index == 0);
        }
        assert_eq!(layout.edges[1].style, mermaid_rs_renderer::EdgeStyle::Dotted);
        assert_eq!(layout.edges[1].override_style.stroke.as_deref(), Some("#ff0000"));
        assert_eq!(layout.edges[1].override_style.stroke_width, Some(3.0));
    }

    #[test]
    fn renders_chinese_flowchart_and_sequence_with_real_labels() {
        for source in [
            "flowchart LR\n A[用户输入] --> B{是否有效}\n B -->|是| C[显示结果]\n B -->|否| D[修改]",
            "sequenceDiagram\n participant U as 用户\n participant A as AI服务\n U->>A: 发送请求\n A-->>U: 返回结果",
        ] {
            for dark in [false, true] {
                let result = render_diagram(source, dark).unwrap();
                assert!(result.starts_with(b"\x89PNG\r\n\x1a\n"));
                assert!(result.len() > 1000);
            }
        }
    }

    #[test]
    fn rejects_invalid_or_oversized_diagrams() {
        assert!(render_diagram("not a diagram", false).is_err());
        assert!(render_diagram(&"A".repeat(65537), false).is_err());
    }

    #[test]
    fn renders_subgraphs_and_sequence_branches() {
        for source in [
            "flowchart TD\nsubgraph 输入\n A[接收请求]\nend\nA --> B{有效}\nB -->|是| C[处理]\nB -->|否| A",
            "flowchart TD\nsubgraph __winone_caption_0[阶段]\n A[开始] -->|执行| B[完成]\nend",
            "sequenceDiagram\nparticipant U as 用户\nparticipant S as 服务\nU->>S: 请求\nactivate S\nalt 成功\nS-->>U: 结果\nelse 失败\nS-->>U: 错误\nend\ndeactivate S\nNote over U,S: 本地处理",
        ] { assert!(render_diagram(source, false).unwrap().len() > 1000); }
    }
}
