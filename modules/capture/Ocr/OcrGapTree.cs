// SPDX-License-Identifier: MIT
// Adapted from hiroi-sora/Umi-OCR 83173efc4f4453b41223ea96d63d44534bf0505c,
// parser_tools/gap_tree.py. Copyright (c) 2023 hiroi-sora.
// WinOne ignores gaps narrower than two line heights to keep word fragments
// together. The column tree and its reading order follow the upstream method.
namespace WinOne.Capture.Ocr;

internal static class OcrGapTree
{
    private sealed record Gap(double Left, double Right, int Start, int End = -1);
    private sealed class Node(double left, double right, int top, int bottom)
    {
        internal double Left = left, Right = right;
        internal int Top = top, Bottom = bottom;
        internal List<OcrBlock> Blocks = [];
        internal List<Node> Children = [];
    }

    internal static List<List<OcrBlock>> Regions(List<OcrBlock> blocks)
    {
        if (blocks.Count == 0) return [];
        var ordered = blocks.OrderBy(block => block.Top).ToArray();
        double pageLeft = blocks.Min(block => block.Left) - 1;
        double pageRight = blocks.Max(block => block.Right) + 1;
        var rows = new List<List<OcrBlock>>();
        var cuts = new List<Gap>();
        var gaps = new List<Gap>();
        for (int index = 0; index < ordered.Length; index++)
        {
            var row = new List<OcrBlock> { ordered[index] };
            double bottom = ordered[index].Bottom;
            while (index + 1 < ordered.Length && ordered[index + 1].Top <= bottom)
                row.Add(ordered[++index]);
            row = row.OrderBy(block => block.Left).ThenBy(block => block.Right).ToList();
            var rowGaps = new List<Gap>();
            double start = pageLeft;
            for (int column = 0; column < row.Count; column++)
            {
                var block = row[column];
                double minimumGap = column == 0 ? 0 : 2 * Math.Max(block.Height, row[column - 1].Height);
                if (block.Left - start > minimumGap) rowGaps.Add(new Gap(start, block.Left, rows.Count));
                start = Math.Max(start, block.Right);
            }
            rowGaps.Add(new Gap(start, pageRight, rows.Count));
            var continued = new List<Gap>();
            var used = new bool[rowGaps.Count];
            foreach (var previous in gaps)
            {
                bool intersects = false;
                for (int next = 0; next < rowGaps.Count; next++)
                {
                    double left = Math.Max(previous.Left, rowGaps[next].Left);
                    double right = Math.Min(previous.Right, rowGaps[next].Right);
                    if (left > right) continue;
                    continued.Add(new Gap(left, right, previous.Start));
                    intersects = used[next] = true;
                }
                if (!intersects) cuts.Add(previous with { End = rows.Count - 1 });
            }
            for (int next = 0; next < rowGaps.Count; next++)
                if (!used[next]) continued.Add(rowGaps[next]);
            gaps = continued;
            rows.Add(row);
        }
        cuts.AddRange(gaps.Select(gap => gap with { End = rows.Count - 1 }));
        cuts = cuts.OrderBy(gap => gap.Left).ToList();
        var rowCuts = rows.Select(_ => new List<Gap>()).ToArray();
        foreach (var cut in cuts)
            for (int row = cut.Start; row <= cut.End; row++) rowCuts[row].Add(cut);

        var root = new Node(cuts[0].Left - 1, cuts[^1].Right + 1, -1, -1);
        var completed = new List<Node> { root };
        var active = new List<Node>();
        void Complete(Node node)
        {
            double right = node.Right - 2;
            var parent = completed.Where(candidate => candidate.Left <= right && right <= candidate.Right + .0001 && candidate.Bottom < node.Top)
                .OrderByDescending(candidate => candidate.Bottom).ThenByDescending(candidate => candidate.Right).First();
            parent.Children.Add(node);
            completed.Add(node);
        }
        for (int rowIndex = 0; rowIndex < rows.Count; rowIndex++)
        {
            var rowGaps = rowCuts[rowIndex];
            var continued = new List<Node>();
            foreach (var node in active)
            {
                bool left = rowGaps.Any(gap => gap.Right == node.Left);
                bool right = rowGaps.Any(gap => gap.Left == node.Right);
                bool split = rowGaps.Any(gap => node.Left < gap.Left && gap.Left < node.Right || node.Left < gap.Right && gap.Right < node.Right);
                if (!left || !right || split) Complete(node);
                else { node.Bottom = rowIndex; continued.Add(node); }
            }
            active = continued;
            int gapIndex = 0;
            foreach (var block in rows[rowIndex])
            {
                while (gapIndex + 2 < rowGaps.Count && block.Left + .0001 > rowGaps[gapIndex + 1].Left) gapIndex++;
                double left = rowGaps[gapIndex].Right, right = rowGaps[gapIndex + 1].Left;
                var node = active.FirstOrDefault(candidate => candidate.Left == left && candidate.Right == right);
                if (node is null)
                {
                    node = new Node(left, right, rowIndex, rowIndex);
                    active.Add(node);
                }
                node.Blocks.Add(block);
            }
        }
        foreach (var node in active) Complete(node);
        var result = new List<List<OcrBlock>>();
        var stack = new Stack<Node>();
        stack.Push(root);
        while (stack.TryPop(out var node))
        {
            if (node.Blocks.Count > 0) result.Add(node.Blocks.OrderBy(block => block.Top).ToList());
            foreach (var child in node.Children.OrderByDescending(child => child.Left)) stack.Push(child);
        }
        return result;
    }
}
