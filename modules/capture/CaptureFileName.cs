using System.Globalization;
using System.Text;
using System.Text.RegularExpressions;

namespace WinOne.Capture;

internal static class CaptureFileName
{
    public static string Render(string? template, DateTime at, int sequence = 1)
    {
        var useDefault = string.IsNullOrEmpty(template);
        if (string.IsNullOrEmpty(template)) template = "WinOne-{timestamp}-OK";
        void Invalid() => throw new InvalidDataException("文件名模板无效，请检查占位符、长度和特殊字符");
        if (Encoding.UTF8.GetByteCount(template) > 160 || sequence < 1) Invalid();
        var name = template.Replace("{timestamp}", at.ToString("yyyyMMddHHmmss", CultureInfo.InvariantCulture))
            .Replace("{date}", at.ToString("yyyyMMdd", CultureInfo.InvariantCulture)).Replace("{time}", at.ToString("HHmmss", CultureInfo.InvariantCulture))
            .Replace("{sequence}", sequence.ToString("D2", CultureInfo.InvariantCulture));
        if (name.Any(c => c < 32 || "{}<>:\"/\\|?*".Contains(c))) Invalid();
        if (name.EndsWith(".png", StringComparison.OrdinalIgnoreCase)) name = name[..^4];
        if (name.Length == 0 || name.EndsWith('.') || name.EndsWith(' ')) Invalid();
        if (Regex.IsMatch(name.Split('.')[0].Trim().ToUpperInvariant(), "^(CON|PRN|AUX|NUL|COM[1-9¹²³]|LPT[1-9¹²³])$", RegexOptions.CultureInvariant)) Invalid();
        if (sequence > 1 && !template.Contains("{sequence}")) name += "-" + (useDefault ? sequence - 1 : sequence).ToString("D2", CultureInfo.InvariantCulture);
        if (Encoding.UTF8.GetByteCount(name) > 240) Invalid();
        return name + ".png";
    }
}
