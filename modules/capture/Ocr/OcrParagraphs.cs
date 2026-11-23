// SPDX-License-Identifier: MIT
// Adapted from hiroi-sora/Umi-OCR 83173efc4f4453b41223ea96d63d44534bf0505c,
// parser_tools/paragraph_parse.py. Copyright (c) 2023 hiroi-sora.
// Preserve the upstream two-pass geometry grouping and Unicode word spacing.
// WinOne additionally keeps heading, list and large vertical-gap boundaries.
using System.Text;
using System.Text.RegularExpressions;

namespace WinOne.Capture.Ocr;

internal static class OcrParagraphs
{
    private const double Threshold = 1.2;
    private static readonly Regex ListStart = new(@"^(?:[-*]\s+|[•●▪◦]\s*|\d+[.)、．](?:\s+|(?=[\u3400-\u9FFF]))|[一二三四五六七八九十]+[、.．]\s*)", RegexOptions.CultureInvariant);
    private static readonly Regex CodeLine = new(@"^(?:[{}]\s*;?$|//|(?:if|for|while|switch)\s*\(|(?:return|const|let|var|def|class|function|public|private|using|import)\b.*[;:{=])", RegexOptions.CultureInvariant);

    internal static bool ContainsCode(IEnumerable<OcrBlock> blocks) => blocks.Any(block => CodeLine.IsMatch(block.Text.Trim()));

    internal static string Compose(IReadOnlyList<OcrBlock> lines)
    {
        if (lines.Count == 0) return "";
        var sections = new List<List<OcrBlock>> { new() { lines[0] } };
        for (int index = 1; index < lines.Count; index++)
        {
            var previous = lines[index - 1];
            var next = lines[index];
            bool heading = Math.Max(previous.Height, next.Height) > Math.Min(previous.Height, next.Height) * 1.3;
            bool gap = next.Top - previous.Bottom > Math.Max(previous.Height, next.Height) * 1.25;
            if (heading || gap || ListStart.IsMatch(next.Text) || previous.Text.EndsWith(':') || previous.Text.EndsWith('：'))
                sections.Add([]);
            sections[^1].Add(next);
        }
        var result = new StringBuilder();
        bool previousList = false;
        foreach (var section in sections)
        {
            bool list = ListStart.IsMatch(section[0].Text);
            if (result.Length > 0) result.Append(list && previousList ? "\n" : "\n\n");
            result.Append(string.Join("\n\n", Group(section).Select(Join)));
            previousList = list;
        }
        return result.ToString();
    }

    private static List<List<OcrBlock>> Group(List<OcrBlock> lines)
    {
        var paragraphs = new List<List<OcrBlock>>();
        var spacings = new List<double?>();
        var current = new List<OcrBlock> { lines[0] };
        double left = lines[0].Left, right = lines[0].Right, bottom = lines[0].Bottom, height = lines[0].Height;
        double? spacing = null;
        foreach (var line in lines.Skip(1))
        {
            double gap = line.Top - bottom;
            double leftTolerance = ListStart.IsMatch(current[0].Text) ? height * (1 + Threshold) : height * Threshold;
            if (Math.Abs(left - line.Left) <= leftTolerance && Math.Abs(right - line.Right) <= height * Threshold
                && (spacing is null || gap < spacing + height * .5))
            {
                left = (left + line.Left) / 2;
                right = (right + line.Right) / 2;
                height = (height + line.Height) / 2;
                spacing = spacing is null ? gap : (spacing + gap) / 2;
                current.Add(line);
            }
            else
            {
                paragraphs.Add(current);
                spacings.Add(spacing);
                current = [line];
                left = line.Left; right = line.Right; height = line.Height; spacing = null;
            }
            bottom = line.Bottom;
        }
        paragraphs.Add(current);
        spacings.Add(spacing);

        // A short last line or indented first line may be a singleton after
        // matching the body margins. Attach it to the closer eligible body.
        for (int index = paragraphs.Count - 1; index >= 0; index--)
        {
            if (paragraphs[index].Count != 1) continue;
            var line = paragraphs[index][0];
            var above = index > 0 ? paragraphs[index - 1][^1] : null;
            var below = index + 1 < paragraphs.Count ? paragraphs[index + 1][0] : null;
            bool up = above is not null && Math.Abs(above.Left - line.Left) <= above.Height * Threshold
                && line.Right <= above.Right + above.Height * Threshold
                && (spacings[index - 1] is null || line.Top - above.Bottom <= spacings[index - 1] + above.Height * .5);
            bool down = below is not null && below.Left - below.Height * Threshold <= line.Left && line.Left <= below.Left + below.Height * (1 + Threshold)
                && (paragraphs[index + 1].Count > 1 ? Math.Abs(below.Right - line.Right) <= below.Height * Threshold : below.Right - below.Height * Threshold < line.Right)
                && (spacings[index + 1] is null || below.Top - line.Bottom <= spacings[index + 1] + below.Height * .5);
            if (!up && !down) continue;
            if (up && (!down || line.Top - above!.Bottom < below!.Top - line.Bottom)) paragraphs[index - 1].Add(line);
            else paragraphs[index + 1].Insert(0, line);
            paragraphs.RemoveAt(index);
            spacings.RemoveAt(index);
        }
        return paragraphs;
    }

    private static string Join(List<OcrBlock> lines)
    {
        var result = new StringBuilder(lines[0].Text);
        string previous = lines[0].Text;
        foreach (var line in lines.Skip(1))
        {
            var left = Rune.GetRuneAt(previous, previous.Length - (char.IsLowSurrogate(previous[^1]) ? 2 : 1));
            var right = Rune.GetRuneAt(line.Text, 0);
            if (!(IsCjk(left.Value) && IsCjk(right.Value)) && left.Value != '-' && !Rune.IsPunctuation(right)) result.Append(' ');
            result.Append(line.Text);
            previous = line.Text;
        }
        return result.ToString();
    }

    private static bool IsCjk(int value) => value is >= 0x4E00 and <= 0x9FFF or >= 0x3040 and <= 0x30FF
        or >= 0x1100 and <= 0x11FF or >= 0x3130 and <= 0x318F or >= 0xAC00 and <= 0xD7AF
        or >= 0x3000 and <= 0x303F or >= 0xFE30 and <= 0xFE4F or >= 0xFF00 and <= 0xFFEF
        or >= 0x3400 and <= 0x4DBF or >= 0x20000 and <= 0x323AF;
}
