use mermaid_rs_renderer::{DiagramKind, Graph, Layout, LayoutConfig, NodeShape, Theme};

struct Caption { before: usize, after: usize, id: String }

/// Reserve text in the graph before routing, as layered layouts do with edge
/// label proxy nodes. Restore the original edges before SVG rendering, so a
/// caption cannot become an extra visible process step or change arrow meaning.
pub(super) fn layout(graph: &Graph, theme: &Theme, config: &LayoutConfig) -> Layout {
    if graph.kind != DiagramKind::Flowchart || !graph.edges.iter().any(|edge| edge.label.is_some()) {
        return mermaid_rs_renderer::compute_layout(graph, theme, config);
    }
    let mut reserved = graph.clone();
    let edges = std::mem::take(&mut reserved.edges);
    let styles = std::mem::take(&mut reserved.edge_styles);
    let mut captions = Vec::new();
    for (index, mut edge) in edges.into_iter().enumerate() {
        let Some(text) = edge.label.take().filter(|label| !label.trim().is_empty()) else {
            if let Some(style) = styles.get(&index) { reserved.edge_styles.insert(reserved.edges.len(), style.clone()); }
            reserved.edges.push(edge);
            continue;
        };
        let mut id = format!("__winone_caption_{index}");
        while reserved.nodes.contains_key(&id) { id.push('_'); }
        // Quoted Mermaid edge captions use the quotes as delimiters, not text.
        let text = text.strip_prefix('"').and_then(|value| value.strip_suffix('"')).unwrap_or(&text).to_string();
        reserved.ensure_node(&id, Some(text), Some(NodeShape::Text));
        for group in &mut reserved.subgraphs {
            if group.nodes.contains(&edge.from) && group.nodes.contains(&edge.to) { group.nodes.push(id.clone()); }
        }
        let mut before = edge.clone();
        before.to = id.clone();
        before.arrow_end = false;
        before.arrow_end_kind = None;
        before.end_decoration = None;
        before.end_label = None;
        edge.from = id.clone();
        edge.arrow_start = false;
        edge.arrow_start_kind = None;
        edge.start_decoration = None;
        edge.start_label = None;
        let first = reserved.edges.len();
        if let Some(style) = styles.get(&index) {
            reserved.edge_styles.insert(first, style.clone());
            reserved.edge_styles.insert(first+1, style.clone());
        }
        captions.push(Caption { before: first, after: first+1, id });
        reserved.edges.push(before);
        reserved.edges.push(edge);
    }
    let mut layout = mermaid_rs_renderer::compute_layout(&reserved, theme, config);
    let mut original_edges = Vec::with_capacity(graph.edges.len());
    let mut captions = captions.into_iter().peekable();
    let mut index = 0;
    while index < layout.edges.len() {
        let mut edge = layout.edges[index].clone();
        if captions.peek().is_some_and(|caption| caption.before == index) {
            let caption = captions.next().unwrap();
            let node = layout.nodes.remove(&caption.id).expect("caption node survives layout");
            let after = &layout.edges[caption.after];
            edge.to = after.to.clone();
            edge.arrow_end = after.arrow_end;
            edge.arrow_end_kind = after.arrow_end_kind;
            edge.end_decoration = after.end_decoration;
            edge.end_label = after.end_label.clone();
            edge.end_label_anchor = after.end_label_anchor;
            edge.label = Some(node.label);
            edge.label_anchor = Some((node.x+node.width/2.0, node.y+node.height/2.0));
            // The joining segment lies inside reserved caption space. The
            // renderer's normal caption background covers that carrying line.
            edge.points.extend_from_slice(&after.points);
            for group in &mut layout.subgraphs { group.nodes.retain(|id| id != &caption.id); }
            index += 1;
        }
        original_edges.push(edge);
        index += 1;
    }
    layout.edges = original_edges;
    layout
}
