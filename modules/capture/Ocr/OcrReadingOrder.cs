// SPDX-License-Identifier: GPL-3.0-only
// Ported from tcita/HushSnap 2d43f0e1fd125484bd132ee6e9cad1db65fb7a8c,
// hushsnap/ocr/ppocr.py and text.py. Geometry, upper medians, spacing,
// paragraph breaks and indentation retain the original rules.
using System.Text;
using System.Text.RegularExpressions;

namespace WinOne.Capture.Ocr;

internal sealed record OcrBlock(string Text, double Left, double Top, double Right, double Bottom)
{
    internal double Width => Right - Left;
    internal double Height => Bottom - Top;
    internal double CenterX => (Left + Right) / 2;
    internal double CenterY => (Top + Bottom) / 2;
}

internal static class OcrReadingOrder
{
    private const double BoxToFont = 1.2;
    private const string Cjk = @"\u2E80-\u2EFF\u2F00-\u2FDF\u3040-\u309F\u30A0-\u30FF\u30FB-\u30FF\u3100-\u312F\u3200-\u32FF\u3400-\u4DBF\u4E00-\u9FFF\uF900-\uFAFF";
    private const string Latin = @"A-Za-z\u0370-\u03FF0-9@$%^&*\-+\\=\|/\u00A1-\u00FF\u2150-\u218F\u2700-\u27BF";
    private static readonly Regex CjkLatin = new($"([{Cjk}])([{Latin}])", RegexOptions.CultureInvariant);
    private static readonly Regex LatinCjk = new($"([{Latin}~!;:,./?])([{Cjk}])", RegexOptions.CultureInvariant);
    private static readonly Regex Url = new(@"https?://?[^\s<>\]\)\}]+", RegexOptions.IgnoreCase | RegexOptions.CultureInvariant);

    private sealed record Line(string Text, List<OcrBlock> Words)
    {
        internal double Left => Words.Min(word => word.Left);
        internal double Height => Median(Words.Select(word => word.Height));
        internal double CenterY => Median(Words.Select(word => word.CenterY));
    }

    private static double Median(IEnumerable<double> values)
    {
        var sorted = values.Order().ToArray();
        return sorted[sorted.Length / 2];
    }

    internal static string Compose(IEnumerable<OcrBlock> input, bool mergeParagraphs = true)
    {
        var blocks = input.Where(block => !string.IsNullOrWhiteSpace(block.Text) && block.Width > 0 && block.Height > 0)
            .Select(block => block with { Text = block.Text.Trim() }).ToList();
        if (blocks.Count == 0) return "";
        var totalArea = blocks.Sum(block => block.Width * block.Height);
        var verticalArea = blocks.Where(block => block.Height > block.Width * 1.5).Sum(block => block.Width * block.Height);
        bool vertical = verticalArea / totalArea >= .5;
        if (mergeParagraphs && !vertical && !OcrParagraphs.ContainsCode(blocks))
        {
            return string.Join("\n\n", OcrGapTree.Regions(blocks).Select(region =>
                OcrParagraphs.Compose(ReadLines(region, false).Select(line => new OcrBlock(line.Text,
                    line.Left, line.Words.Min(word => word.Top), line.Words.Max(word => word.Right), line.Words.Max(word => word.Bottom))).ToArray())));
        }
        var lines = ReadLines(blocks, vertical);
        if (vertical || lines.Length == 1) return string.Join('\n', lines.Select(line => line.Text)).TrimEnd();

        double baseline = lines.Min(line => line.Left);
        bool Indented(Line line) => (line.Left - baseline) / (line.Height / BoxToFont) > 1;
        var offsets = lines.Where(Indented).Select(line => Math.Round(line.Left) - baseline).Distinct().Order().ToArray();
        var result = new List<string>();
        for (int index = 0; index < lines.Length; index++)
        {
            var line = lines[index];
            int level = offsets.Length > 0 && Indented(line) ? Math.Max(1, (int)Math.Round((Math.Round(line.Left) - baseline) / offsets[0])) : 0;
            result.Add(new string(' ', level * 4) + line.Text);
            if (index + 1 == lines.Length) continue;
            var next = lines[index + 1];
            double currentHeight = line.Height / BoxToFont, nextHeight = next.Height / BoxToFont;
            if (next.CenterY - line.CenterY > currentHeight / 2 + nextHeight / 2 + 1.5 * Math.Max(currentHeight, nextHeight))
                result.Add("");
        }
        return string.Join('\n', result).TrimEnd();
    }

    private static Line[] ReadLines(List<OcrBlock> blocks, bool vertical)
    {
        var ordered = vertical ? blocks.OrderByDescending(block => block.Right) : blocks.OrderBy(block => block.Top);
        var clusters = new List<List<OcrBlock>>();
        foreach (var block in ordered)
        {
            var current = clusters.LastOrDefault();
            double center = vertical ? block.CenterX : block.CenterY;
            if (current is null || Math.Abs(center - Median(current.Select(item => vertical ? item.CenterX : item.CenterY)))
                >= .4 * Median(current.Select(item => vertical ? item.Width : item.Height)) / BoxToFont)
            {
                current = [];
                clusters.Add(current);
            }
            current.Add(block);
        }
        var sortedClusters = clusters.OrderBy(cluster => vertical ? -cluster.Average(block => block.CenterX) : cluster.Average(block => block.CenterY));
        return sortedClusters.Select(cluster => BuildLine(cluster, vertical)).ToArray();
    }

    private static Line BuildLine(List<OcrBlock> cluster, bool vertical)
    {
        var words = cluster.OrderBy(block => vertical ? block.Top : block.Left).ToList();
        var text = new StringBuilder();
        OcrBlock? previous = null;
        foreach (var block in words)
        {
            if (previous is not null)
            {
                string separator = Separator(previous.Text, block.Text);
                double gap = vertical ? block.Top - previous.Bottom : block.Left - previous.Right;
                double scale = (vertical ? Math.Min(previous.Width, block.Width) : Math.Min(previous.Height, block.Height)) / BoxToFont;
                if (gap / scale > 2) separator += new string(' ', Math.Max(1, (int)Math.Round(gap / scale)));
                text.Append(separator);
            }
            text.Append(block.Text);
            previous = block;
        }
        return new Line(SpaceOutsideUrls(text.ToString()), words);
    }

    private static bool Fullwidth(char character) => character is >= '\u3000' and <= '\u303F'
        or >= '\u3040' and <= '\u30FF' or >= '\u4E00' and <= '\u9FFF'
        or >= '\uAC00' and <= '\uD7AF' or >= '\uFF00' and <= '\uFFEF';

    private static string Separator(string left, string right)
    {
        if (Fullwidth(left[^1]) && Fullwidth(right[0])) return "";
        if ("-—–…」』".Contains(left[^1]) || ",.;:!?)]}，。、；：！？）】》〉〕｝…—–·\"'”’」』".Contains(right[0])) return "";
        return " ";
    }

    private static string SpaceOutsideUrls(string text)
    {
        string Space(string part) => LatinCjk.Replace(CjkLatin.Replace(part, "$1 $2"), "$1 $2");
        var output = new StringBuilder();
        int start = 0;
        foreach (Match match in Url.Matches(text))
        {
            output.Append(Space(text[start..match.Index]));
            output.Append(match.Value);
            start = match.Index + match.Length;
        }
        output.Append(Space(text[start..]));
        return output.ToString();
    }
}
