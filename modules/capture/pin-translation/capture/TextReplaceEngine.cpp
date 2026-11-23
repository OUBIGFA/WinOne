// Tools3000 v1.4.1. The pin translator uses the complete extractStyle path.
// The independent text editor and its network model downloader are outside this feature.
#include "capture/TextReplaceEngine.h"
#include "core/utils/WinUtils.h"
#include <windows.h>
#include <winhttp.h>
#include <d2d1.h>
#include <dwrite.h>
#include <wincodec.h>
#include <wrl/client.h>
#include <opencv2/imgproc.hpp>
#include <opencv2/photo.hpp>

#include <algorithm>
#include <array>
#include <cmath>
#include <deque>
#include <fstream>
#include <memory>
#include <numeric>
#include <unordered_map>

using Microsoft::WRL::ComPtr;
namespace tools3000::capture {

namespace {
void ensureComInitOnThread() noexcept {
    thread_local bool s_inited = false;
    if (!s_inited) {
        const HRESULT hr = CoInitializeEx(nullptr, COINIT_MULTITHREADED);
        if (SUCCEEDED(hr) || hr == RPC_E_CHANGED_MODE) {
            s_inited = true;
        }
    }
}

cv::Mat ensureBgr3Mat(const cv::Mat& src) {
    if (src.empty()) return {};
    if (src.channels() == 3) return src;
    cv::Mat out;
    if (src.channels() == 4) {
        cv::cvtColor(src, out, cv::COLOR_BGRA2BGR);
    } else if (src.channels() == 1) {
        cv::cvtColor(src, out, cv::COLOR_GRAY2BGR);
    }
    return out;
}

bool containsCjkCodepoint(const std::wstring& s) noexcept {
    for (wchar_t ch : s) {
        if ((ch >= 0x4E00 && ch <= 0x9FFF) ||
            (ch >= 0x3400 && ch <= 0x4DBF) ||
            (ch >= 0x3000 && ch <= 0x303F) ||
            (ch >= 0xFF00 && ch <= 0xFFEF)) {
            return true;
        }
    }
    return false;
}

bool containsAsciiAscenderOrDigit(const std::wstring& s) noexcept {
    for (wchar_t ch : s) {
        if ((ch >= L'A' && ch <= L'Z') || (ch >= L'0' && ch <= L'9') ||
            ch == L'b' || ch == L'd' || ch == L'f' || ch == L'h' ||
            ch == L'k' || ch == L'l' || ch == L't') {
            return true;
        }
    }
    return false;
}

// 解 3x3 对称线性方程组 A * x = b（高斯消元，稳定可靠）
bool solveLinear3x3(double A[3][3], double b[3], double x[3]) noexcept {
    double M[3][4];
    for (int r = 0; r < 3; ++r) {
        for (int c = 0; c < 3; ++c) M[r][c] = A[r][c];
        M[r][3] = b[r];
    }
    for (int col = 0; col < 3; ++col) {
        int pivot = col;
        for (int r = col + 1; r < 3; ++r) {
            if (std::abs(M[r][col]) > std::abs(M[pivot][col])) pivot = r;
        }
        if (std::abs(M[pivot][col]) < 1e-8) return false;
        if (pivot != col) {
            for (int c = col; c < 4; ++c) std::swap(M[col][c], M[pivot][c]);
        }
        const double invP = 1.0 / M[col][col];
        for (int c = col; c < 4; ++c) M[col][c] *= invP;
        for (int r = 0; r < 3; ++r) {
            if (r == col) continue;
            const double f = M[r][col];
            for (int c = col; c < 4; ++c) M[r][c] -= f * M[col][c];
        }
    }
    for (int r = 0; r < 3; ++r) x[r] = M[r][3];
    return true;
}

/// 拟合局部二维线性渐变背景平面，自动剔除边框/相邻文字离群像素
LinearBgPlane fitRobustLinearBgPlane(const cv::Mat& bgr, const cv::Rect& roi) {
    LinearBgPlane plane{};
    plane.originX = static_cast<float>(roi.x) + static_cast<float>(roi.width) * 0.5f;
    plane.originY = static_cast<float>(roi.y) + static_cast<float>(roi.height) * 0.5f;
    if (bgr.empty() || roi.width <= 0 || roi.height <= 0) return plane;

    struct Sample {
        float dx, dy;
        cv::Vec3b px;
        bool isInnerRing = false; // 紧贴 ROI 外围 1~2px 的近环样本（紧凑表格/单字选区防远环撞入上下行黑字）
    };
    std::vector<Sample> samples;
    samples.reserve(512);

    auto pushPt = [&](int x, int y, bool innerRing) {
        if (x < 0 || x >= bgr.cols || y < 0 || y >= bgr.rows) return;
        samples.push_back(Sample{
            static_cast<float>(x) - plane.originX,
            static_cast<float>(y) - plane.originY,
            bgr.at<cv::Vec3b>(y, x),
            innerRing
        });
    };

    // 采样外围近环 (1~2px) + 远环 (4~6px)（近环精准命中紧凑 Excel 单元格留白，远环稳定拟合渐变斜率）
    const int ringMax = 6;
    const int xStart = (std::max)(0, roi.x - ringMax);
    const int xEnd = (std::min)(bgr.cols - 1, roi.x + roi.width + ringMax);
    const int yStart = (std::max)(0, roi.y - ringMax);
    const int yEnd = (std::min)(bgr.rows - 1, roi.y + roi.height + ringMax);

    const int stepX = (std::max)(1, (xEnd - xStart) / 48);
    const int stepY = (std::max)(1, (yEnd - yStart) / 14);

    constexpr std::array<int, 4> kRingDists = {{1, 2, 4, 6}};
    for (int x = xStart; x <= xEnd; x += stepX) {
        for (int dy : kRingDists) {
            const bool inner = (dy <= 2);
            pushPt(x, roi.y - dy, inner);
            pushPt(x, roi.y + roi.height - 1 + dy, inner);
        }
    }
    for (int y = (std::max)(0, roi.y); y < (std::min)(bgr.rows, roi.y + roi.height); y += stepY) {
        for (int dx : kRingDists) {
            const bool inner = (dx <= 2);
            pushPt(roi.x - dx, y, inner);
            pushPt(roi.x + roi.width - 1 + dx, y, inner);
        }
    }

    // 极端贴边兜底：采样 ROI 四条内缘
    if (samples.size() < 12) {
        for (int x = (std::max)(0, roi.x); x < (std::min)(bgr.cols, roi.x + roi.width); x += (std::max)(1, roi.width / 24)) {
            pushPt(x, (std::max)(0, roi.y), true);
            pushPt(x, (std::min)(bgr.rows - 1, roi.y + roi.height - 1), true);
        }
    }
    if (samples.empty()) return plane;

    // 1. 量化颜色主峰众数锚定（Unimodal Dominant Mode）+ 次峰对抗检测：
    //    相比简单中位数，主峰众数在四周挤满 40%~50% 单侧黑字（如紧凑 Excel 表格单字）时绝不被拉灰，
    //    同时通过检测远离主峰的次峰强度，100% 识别 50/50 棋盘格/双色条纹等复杂底纹！
    std::unordered_map<uint32_t, int> bucketCounts;
    bucketCounts.reserve(64);
    auto colorBucketKey = [](const cv::Vec3b& c) noexcept -> uint32_t {
        return (static_cast<uint32_t>(c[0] >> 4) << 8) |
               (static_cast<uint32_t>(c[1] >> 4) << 4) |
               static_cast<uint32_t>(c[2] >> 4);
    };
    for (const auto& s : samples) {
        ++bucketCounts[colorBucketKey(s.px)];
    }

    uint32_t bestKey = 0;
    int bestBucketCnt = 0;
    for (const auto& kv : bucketCounts) {
        if (kv.second > bestBucketCnt) {
            bestBucketCnt = kv.second;
            bestKey = kv.first;
        }
    }
    const int peakB0 = static_cast<int>((bestKey >> 8) & 0xFu) * 16 + 8;
    const int peakG0 = static_cast<int>((bestKey >> 4) & 0xFu) * 16 + 8;
    const int peakR0 = static_cast<int>(bestKey & 0xFu) * 16 + 8;

    // 统计远离主峰 (> 32 色差) 的最强次峰（专门拦截 50/50 棋盘格与双色条纹）
    int secondPeakCnt = 0;
    for (const auto& kv : bucketCounts) {
        const int kb = static_cast<int>((kv.first >> 8) & 0xFu) * 16 + 8;
        const int kg = static_cast<int>((kv.first >> 4) & 0xFu) * 16 + 8;
        const int kr = static_cast<int>(kv.first & 0xFu) * 16 + 8;
        const int distToPeak = (std::max)({std::abs(kb - peakB0), std::abs(kg - peakG0), std::abs(kr - peakR0)});
        if (distToPeak >= 32 && kv.second > secondPeakCnt) {
            secondPeakCnt = kv.second;
        }
    }
    const float secondPeakRatio = static_cast<float>(secondPeakCnt) / static_cast<float>((std::max<size_t>)(1, samples.size()));

    // 在主峰邻域 (<= 24 色差) 内求精确三通道中位数，得到零拉偏的真实背景基准色
    std::vector<uint8_t> bv, gv, rv;
    bv.reserve(samples.size());
    gv.reserve(samples.size());
    rv.reserve(samples.size());
    for (const auto& s : samples) {
        const int d = (std::max)({
            std::abs(static_cast<int>(s.px[0]) - peakB0),
            std::abs(static_cast<int>(s.px[1]) - peakG0),
            std::abs(static_cast<int>(s.px[2]) - peakR0)
        });
        if (d <= 24) {
            bv.push_back(s.px[0]);
            gv.push_back(s.px[1]);
            rv.push_back(s.px[2]);
        }
    }
    if (bv.size() < 6) {
        bv.clear(); gv.clear(); rv.clear();
        for (const auto& s : samples) {
            bv.push_back(s.px[0]);
            gv.push_back(s.px[1]);
            rv.push_back(s.px[2]);
        }
    }
    const size_t mid = bv.size() / 2;
    std::nth_element(bv.begin(), bv.begin() + static_cast<ptrdiff_t>(mid), bv.end());
    std::nth_element(gv.begin(), gv.begin() + static_cast<ptrdiff_t>(mid), gv.end());
    std::nth_element(rv.begin(), rv.begin() + static_cast<ptrdiff_t>(mid), rv.end());
    const int medB = bv[mid], medG = gv[mid], medR = rv[mid];

    std::vector<int> dists;
    dists.reserve(samples.size());
    for (const auto& s : samples) {
        const int d = (std::max)({
            std::abs(static_cast<int>(s.px[0]) - medB),
            std::abs(static_cast<int>(s.px[1]) - medG),
            std::abs(static_cast<int>(s.px[2]) - medR)
        });
        dists.push_back(d);
    }
    std::vector<int> sortedDists = dists;
    const size_t p50Idx = sortedDists.size() / 2;
    std::nth_element(sortedDists.begin(), sortedDists.begin() + static_cast<ptrdiff_t>(p50Idx), sortedDists.end());
    // 采用鲁棒统计学 MAD (中位数绝对偏差) 设定内点阈值，防紧凑表格单字四周邻字污染
    const int inlierThresh = (std::max)(14, (std::min)(22, sortedDists[p50Idx] * 2 + 6));

    std::vector<Sample> inliers;
    inliers.reserve(samples.size());
    for (size_t i = 0; i < samples.size(); ++i) {
        if (dists[i] <= inlierThresh) {
            inliers.push_back(samples[i]);
        }
    }
    if (inliers.size() < 6) {
        inliers = samples;
    }

    // 2. 最小二乘平面拟合（加微弱岭正则防病态共线）
    double A[3][3]{};
    double rhs[3][3]{};
    for (const auto& s : inliers) {
        const double v[3] = {1.0, static_cast<double>(s.dx), static_cast<double>(s.dy)};
        for (int r = 0; r < 3; ++r) {
            for (int c = 0; c < 3; ++c) A[r][c] += v[r] * v[c];
            for (int ch = 0; ch < 3; ++ch) rhs[ch][r] += v[r] * static_cast<double>(s.px[ch]);
        }
    }
    A[1][1] += 1e-3;
    A[2][2] += 1e-3;

    for (int ch = 0; ch < 3; ++ch) {
        double ACopy[3][3];
        double bCopy[3];
        double sol[3]{};
        std::memcpy(ACopy, A, sizeof(A));
        std::memcpy(bCopy, rhs[ch], sizeof(bCopy));
        if (solveLinear3x3(ACopy, bCopy, sol)) {
            plane.c0[ch] = std::clamp(static_cast<float>(sol[0]), 0.0f, 255.0f);
            plane.cx[ch] = std::clamp(static_cast<float>(sol[1]), -1.5f, 1.5f);
            plane.cy[ch] = std::clamp(static_cast<float>(sol[2]), -1.5f, 1.5f);
        } else {
            plane.c0[ch] = static_cast<float>(ch == 0 ? medB : (ch == 1 ? medG : medR));
            plane.cx[ch] = 0.0f;
            plane.cy[ch] = 0.0f;
        }
    }

    // 3. 评估纯净背景主导度：
    //    情形 A：全环内点占绝对主导 (>= 64%) 且紧贴二维线性渐变平面 (inlierStd <= 8.5)；
    //    情形 B：紧凑单字表格（四周 4~6px 远环全撞入紧邻黑字）中，紧贴 1~2px 近环内点占优 (>= 66%) 且内点极其纯净 (inlierStd <= 5.5)；
    //    二者均完美豁免 50/50 棋盘格（近远环均为 50%）与复杂纹理照片（inlierStd > 10）。
    double inlierSqSum = 0.0;
    size_t innerRingTotal = 0;
    size_t innerRingInliers = 0;
    for (size_t i = 0; i < samples.size(); ++i) {
        if (samples[i].isInnerRing) {
            ++innerRingTotal;
            if (dists[i] <= inlierThresh) ++innerRingInliers;
        }
    }
    for (const auto& s : inliers) {
        const cv::Vec3b pred = plane.evalAt(plane.originX + s.dx, plane.originY + s.dy);
        const double db = static_cast<double>(s.px[0]) - static_cast<double>(pred[0]);
        const double dg = static_cast<double>(s.px[1]) - static_cast<double>(pred[1]);
        const double dr = static_cast<double>(s.px[2]) - static_cast<double>(pred[2]);
        inlierSqSum += (db * db + dg * dg + dr * dr) / 3.0;
    }
    const float inlierStd = static_cast<float>(std::sqrt(inlierSqSum / static_cast<double>((std::max<size_t>)(1, inliers.size()))));
    const float inlierRatio = static_cast<float>(inliers.size()) / static_cast<float>((std::max<size_t>)(1, samples.size()));
    const float innerRingRatio = (innerRingTotal >= 8)
        ? (static_cast<float>(innerRingInliers) / static_cast<float>(innerRingTotal))
        : inlierRatio;

    // 紧凑单字表格辅助校验：单字四周 1~6px 均紧邻黑字时，文字框内部本身 58%~75% 仍是纯净底色；
    // 50/50 棋盘格或复杂纹理在框内叠加文字后单色匹配率必然 < 50%，绝不误判！
    float roiMatchRatio = 0.0f;
    if (inlierRatio >= 0.36f && inlierRatio < 0.64f && inlierStd <= 4.5f) {
        const cv::Rect safeInner = roi & cv::Rect(0, 0, bgr.cols, bgr.rows);
        if (safeInner.width > 0 && safeInner.height > 0) {
            size_t totalInner = 0;
            size_t matchedInner = 0;
            for (int y = safeInner.y; y < safeInner.y + safeInner.height; ++y) {
                const auto* row = bgr.ptr<cv::Vec3b>(y);
                for (int x = safeInner.x; x < safeInner.x + safeInner.width; ++x) {
                    const cv::Vec3b pred = plane.evalAt(static_cast<float>(x), static_cast<float>(y));
                    const int d = (std::max)({
                        std::abs(static_cast<int>(row[x][0]) - static_cast<int>(pred[0])),
                        std::abs(static_cast<int>(row[x][1]) - static_cast<int>(pred[1])),
                        std::abs(static_cast<int>(row[x][2]) - static_cast<int>(pred[2]))
                    });
                    ++totalInner;
                    if (d <= inlierThresh) ++matchedInner;
                }
            }
            if (totalInner > 0) {
                roiMatchRatio = static_cast<float>(matchedInner) / static_cast<float>(totalInner);
            }
        }
    }

    // 单峰纯净背景判定：要求不存在 >= 28% 的强异色次峰（直接拦杀 50/50 棋盘格与双色条纹）
    const bool unimodalBg = (secondPeakRatio < 0.28f);
    const bool dominantCleanPlane =
        (inlierRatio >= 0.64f && inlierStd <= 8.5f) ||
        (unimodalBg && inlierRatio >= 0.42f && innerRingRatio >= 0.52f && inlierStd <= 8.5f) ||
        (unimodalBg && inlierRatio >= 0.36f && roiMatchRatio >= 0.50f && inlierStd <= 8.5f);
    if (dominantCleanPlane) {
        plane.complexity = inlierStd;
    } else {
        double allSqSum = 0.0;
        for (const auto& s : samples) {
            const cv::Vec3b pred = plane.evalAt(plane.originX + s.dx, plane.originY + s.dy);
            const double db = static_cast<double>(s.px[0]) - static_cast<double>(pred[0]);
            const double dg = static_cast<double>(s.px[1]) - static_cast<double>(pred[1]);
            const double dr = static_cast<double>(s.px[2]) - static_cast<double>(pred[2]);
            allSqSum += (db * db + dg * dg + dr * dr) / 3.0;
        }
        plane.complexity = (std::max)(inlierStd, static_cast<float>(std::sqrt(allSqSum / static_cast<double>((std::max<size_t>)(1, samples.size())))));
    }
    return plane;
}

/// 三阶像素级连通域主权护栏（MECE 覆盖左右跨界连笔、邻字独立点画/偏旁、上下行探入、低分粘连谷底切分、单字符豁免）
struct NeighborGlyphGuardResult {
    cv::Rect obsRoi;
    cv::Mat protectMaskInObs; // CV_8UC1 (obsRoi 尺寸): 255 = 必须保全的相邻未选中字形及其 1px 护城河
    int rightProtectedMinX = INT_MAX; // 若右侧邻字渗入选区并受保护，记录其全图最左 X（供同行右尾顺延无撕裂完整吸纳）
    bool hasProtected = false;

    /// 将保护掩码裁切映射到任意目标区域 dstRoi（全图坐标）
    cv::Mat cropTo(const cv::Rect& dstRoi) const {
        cv::Mat out(dstRoi.height, dstRoi.width, CV_8UC1, cv::Scalar(0));
        if (!hasProtected || protectMaskInObs.empty()) return out;
        const cv::Rect inter = obsRoi & dstRoi;
        if (inter.width <= 0 || inter.height <= 0) return out;
        const cv::Rect srcSub(inter.x - obsRoi.x, inter.y - obsRoi.y, inter.width, inter.height);
        const cv::Rect dstSub(inter.x - dstRoi.x, inter.y - dstRoi.y, inter.width, inter.height);
        protectMaskInObs(srcSub).copyTo(out(dstSub));
        return out;
    }
};

NeighborGlyphGuardResult buildNeighborGlyphGuard(const cv::Mat& bgr,
                                                 const cv::Rect& roi,
                                                 const LinearBgPlane& bgPlane) {
    NeighborGlyphGuardResult res{};
    if (bgr.empty() || roi.width <= 0 || roi.height <= 0) return res;

    const cv::Rect canvasRc(0, 0, bgr.cols, bgr.rows);
    const cv::Rect safeRoi = roi & canvasRc;
    if (safeRoi.width < 2 || safeRoi.height < 2) return res;

    // 观察窗横向外扩 1.5x 行高（足以完整囊括紧邻整字如 "余"/"待"），纵向外扩 0.5x 行高（囊括紧邻上下行）
    const int marginX = std::clamp((safeRoi.height * 3) / 2, 16, 64);
    const int marginY = std::clamp(safeRoi.height / 2, 4, 20);
    res.obsRoi = cv::Rect(
        safeRoi.x - marginX, safeRoi.y - marginY,
        safeRoi.width + marginX * 2, safeRoi.height + marginY * 2) & canvasRc;
    if (res.obsRoi.width <= safeRoi.width && res.obsRoi.height <= safeRoi.height) return res;

    const int inX0 = safeRoi.x - res.obsRoi.x;
    const int inY0 = safeRoi.y - res.obsRoi.y;
    const int inX1 = inX0 + safeRoi.width;
    const int inY1 = inY0 + safeRoi.height;

    cv::Mat diffObs(res.obsRoi.height, res.obsRoi.width, CV_8UC1, cv::Scalar(0));
    int maxInnerDiff = 0;
    for (int r = 0; r < res.obsRoi.height; ++r) {
        const int gy = res.obsRoi.y + r;
        const auto* row = bgr.ptr<cv::Vec3b>(gy) + res.obsRoi.x;
        uint8_t* dRow = diffObs.ptr<uint8_t>(r);
        const bool yInside = (r >= inY0 && r < inY1);
        for (int c = 0; c < res.obsRoi.width; ++c) {
            const cv::Vec3b bg = bgPlane.evalAt(static_cast<float>(res.obsRoi.x + c), static_cast<float>(gy));
            const int d = (std::max)({
                std::abs(static_cast<int>(row[c][0]) - static_cast<int>(bg[0])),
                std::abs(static_cast<int>(row[c][1]) - static_cast<int>(bg[1])),
                std::abs(static_cast<int>(row[c][2]) - static_cast<int>(bg[2]))
            });
            dRow[c] = static_cast<uint8_t>((std::min)(255, d));
            if (yInside && c >= inX0 && c < inX1 && d > maxInnerDiff) {
                maxInnerDiff = d;
            }
        }
    }
    if (maxInnerDiff < 20) return res;

    const uint8_t fgCut = static_cast<uint8_t>(std::clamp((maxInnerDiff * 24) / 100, 16, 48));
    cv::Mat obsFg;
    cv::threshold(diffObs, obsFg, fgCut, 255, cv::THRESH_BINARY);

    cv::Mat labels, stats, centroids;
    const int numLabels = cv::connectedComponentsWithStats(obsFg, labels, stats, centroids, 8, CV_32S);
    if (numLabels <= 1) return res;

    struct CompInfo {
        int left = 0;
        int top = 0;
        int right = 0;
        int bottom = 0;
        int area = 0;
        int insideCnt = 0;
        int leftOutCnt = 0;
        int rightOutCnt = 0;
        int topOutCnt = 0;
        int botOutCnt = 0;
        int insideMinX = INT_MAX;
        int insideMaxX = -1;
        double cx = 0.0;
        double cy = 0.0;
        bool touchesRoi = false;
        bool fullyInside = false;
        bool isNeighbor = false;
        bool isFusedBoundary = false; // 邻字与目标字低分物理粘连，改走列谷底硬切
    };

    std::vector<CompInfo> comps(static_cast<size_t>(numLabels));
    int totalInnerFgPixels = 0;
    int leftInvadeMaxX = -1;
    int rightInvadeMinX = INT_MAX;

    // Pass 1：统计跨界连通域主权（连笔跨左右界、跨上下行界、项目符号/表格线）
    for (int id = 1; id < numLabels; ++id) {
        auto& ci = comps[static_cast<size_t>(id)];
        ci.left = stats.at<int>(id, cv::CC_STAT_LEFT);
        ci.top = stats.at<int>(id, cv::CC_STAT_TOP);
        const int cw = stats.at<int>(id, cv::CC_STAT_WIDTH);
        const int ch = stats.at<int>(id, cv::CC_STAT_HEIGHT);
        ci.right = ci.left + cw;
        ci.bottom = ci.top + ch;
        ci.area = stats.at<int>(id, cv::CC_STAT_AREA);
        ci.cx = centroids.at<double>(id, 0);
        ci.cy = centroids.at<double>(id, 1);

        if (ci.area < 2) continue;
        if (ci.right <= inX0 || ci.left >= inX1 || ci.bottom <= inY0 || ci.top >= inY1) continue;
        // 忽略横跨整窗的超大背景噪块/大框
        if (cw > safeRoi.width + marginX && ch > safeRoi.height + marginY) continue;

        ci.touchesRoi = true;
        ci.fullyInside = (ci.left >= inX0 && ci.right <= inX1 && ci.top >= inY0 && ci.bottom <= inY1);

        if (ci.fullyInside) {
            ci.insideCnt = ci.area;
            ci.insideMinX = ci.left;
            ci.insideMaxX = ci.right - 1;
            totalInnerFgPixels += ci.insideCnt;
            continue;
        }

        for (int r = ci.top; r < ci.bottom; ++r) {
            const int* lRow = labels.ptr<int>(r);
            const bool yIn = (r >= inY0 && r < inY1);
            for (int c = ci.left; c < ci.right; ++c) {
                if (lRow[c] != id) continue;
                const bool xIn = (c >= inX0 && c < inX1);
                if (xIn && yIn) {
                    ++ci.insideCnt;
                    if (c < ci.insideMinX) ci.insideMinX = c;
                    if (c > ci.insideMaxX) ci.insideMaxX = c;
                } else {
                    if (c < inX0) ++ci.leftOutCnt;
                    else if (c >= inX1) ++ci.rightOutCnt;
                    if (r < inY0) ++ci.topOutCnt;
                    else if (r >= inY1) ++ci.botOutCnt;
                }
            }
        }
        totalInnerFgPixels += ci.insideCnt;
        if (ci.insideCnt <= 0) continue;

        const int outsideCnt = ci.area - ci.insideCnt;
        const float outRatio = static_cast<float>(outsideCnt) / static_cast<float>((std::max)(1, ci.area));
        const int insideSpanW = (ci.insideMaxX >= ci.insideMinX) ? (ci.insideMaxX - ci.insideMinX + 1) : 0;

        // 安全阀 1（低分物理粘连检测）：若单个跨界连通域把选区内超过 55% 的宽度连成一片，说明邻字和目标字发生了粘连，
        // 不得整块保护（否则目标字擦不掉），标记为粘连态在后文用列投影谷底精确切分。
        if (insideSpanW > (safeRoi.width * 55) / 100 && ci.insideCnt >= 10 &&
            (ci.leftOutCnt >= 4 || ci.rightOutCnt >= 4)) {
            ci.isFusedBoundary = true;
            continue;
        }

        const int thinRadicalMaxSpan = (std::max)(3, safeRoi.height / 5);
        const bool wideEnoughForThinRadical = (safeRoi.width >= (std::max)(12, (safeRoi.height * 7) / 10));
        const bool isLeftNeighbor = (ci.leftOutCnt > 0) &&
            (outRatio >= 0.36f ||
             (ci.cx < static_cast<double>(inX0) && ci.leftOutCnt >= 3) ||
             (wideEnoughForThinRadical && ci.leftOutCnt >= 3 && outRatio >= 0.22f && insideSpanW <= thinRadicalMaxSpan));
        const bool isRightNeighbor = (ci.rightOutCnt > 0) &&
            (outRatio >= 0.36f ||
             (ci.cx >= static_cast<double>(inX1) && ci.rightOutCnt >= 3) ||
             (wideEnoughForThinRadical && ci.rightOutCnt >= 3 && outRatio >= 0.22f && insideSpanW <= thinRadicalMaxSpan));
        const bool isVertNeighbor = (ci.topOutCnt > 0 || ci.botOutCnt > 0) &&
            (outRatio >= 0.44f ||
             (ci.cy < static_cast<double>(inY0) && ci.topOutCnt >= 3) ||
             (ci.cy >= static_cast<double>(inY1) && ci.botOutCnt >= 3));

        if (isLeftNeighbor || isRightNeighbor || isVertNeighbor) {
            ci.isNeighbor = true;
            if (isLeftNeighbor && ci.insideMaxX >= inX0 && ci.insideMaxX < inX0 + (safeRoi.width * 45) / 100) {
                leftInvadeMaxX = (std::max)(leftInvadeMaxX, ci.insideMaxX);
            }
            if (isRightNeighbor && ci.insideMinX < inX1 && ci.insideMinX > inX0 + (safeRoi.width * 55) / 100) {
                rightInvadeMinX = (std::min)(rightInvadeMinX, ci.insideMinX);
            }
        }
    }

    if (totalInnerFgPixels <= 0) return res;

    // Pass 2：邻字独立点画/偏旁包络守护（专治汉字如 "余" 右下点、"太/犬" 独立点、"氵/彳" 独立首点被整颗包入选区的情形）
    // 安全阀 2（单字符/标点豁免）：仅当选区足够宽（非单独选中窄标点/单数字）时启用
    const bool allowDetachedDotCheck = (safeRoi.width >= (std::max)(12, (safeRoi.height * 7) / 10));
    if (allowDetachedDotCheck) {
        // 统计选区内每列有效墨迹数（仅含非邻居像素），以及紧贴左/右外侧是否存在外部邻字墨迹
        std::vector<int> obsColInk(static_cast<size_t>(res.obsRoi.width), 0);
        for (int r = inY0; r < inY1; ++r) {
            const int* lRow = labels.ptr<int>(r);
            for (int c = 0; c < res.obsRoi.width; ++c) {
                if (lRow[c] > 0) ++obsColInk[static_cast<size_t>(c)];
            }
        }

        // 查找紧贴左界外侧最近的外部墨迹列 & 紧贴右界外侧最近的外部墨迹列
        int nearestExtLeftCol = -1;
        for (int c = inX0 - 1; c >= (std::max)(0, inX0 - 5); --c) {
            if (obsColInk[static_cast<size_t>(c)] > 0) {
                nearestExtLeftCol = c;
                break;
            }
        }
        int nearestExtRightCol = -1;
        for (int c = inX1; c < (std::min)(res.obsRoi.width, inX1 + 5); ++c) {
            if (obsColInk[static_cast<size_t>(c)] > 0) {
                nearestExtRightCol = c;
                break;
            }
        }

        const int maxDotW = (std::max)(4, (safeRoi.height * 46) / 100);
        const int maxDotH = (std::max)(5, (safeRoi.height * 54) / 100);
        const int maxGapScan = (std::max)(3, safeRoi.height / 3);

        for (int id = 1; id < numLabels; ++id) {
            auto& ci = comps[static_cast<size_t>(id)];
            if (!ci.fullyInside || ci.isNeighbor) continue;
            const int cw = ci.right - ci.left;
            const int ch = ci.bottom - ci.top;
            if (cw > maxDotW || ch > maxDotH) continue;
            // 不能占走选区超过 32% 的总墨迹，防误伤短目标自身主干
            if (ci.insideCnt * 100 > totalInnerFgPixels * 32) continue;

            // 2a) 左侧独立点画：
            //   情形 A：位于左侧跨界邻居横向侵入包络 [inX0, leftInvadeMaxX + 2] 之内（例如 "余" 人字头捺下方的右下角点）
            const bool insideLeftEnvelope =
                (leftInvadeMaxX >= inX0 && ci.left <= leftInvadeMaxX + 1 && ci.right <= leftInvadeMaxX + 3);
            //   情形 B：紧贴选区左缘（距左界 <= 2px），紧邻左侧外部汉字（<= 4px），且其右侧存在完整空白列把它与目标主体隔开，
            //           并且该点明显比到右侧目标主体更贴近左侧外部邻居
            bool detachedLeftEdgeDot = false;
            if (!insideLeftEnvelope && nearestExtLeftCol >= 0 &&
                ci.left <= inX0 + 2 && ci.right < inX0 + (safeRoi.width * 42) / 100) {
                int distToExtLeft = ci.left - nearestExtLeftCol;
                int distToNextInner = -1;
                for (int c = ci.right; c < (std::min)(inX1, ci.right + maxGapScan + 4); ++c) {
                    if (obsColInk[static_cast<size_t>(c)] > 0) {
                        distToNextInner = c - ci.right + 1;
                        break;
                    }
                }
                if (distToNextInner >= 2 && distToExtLeft <= 4 && distToExtLeft <= distToNextInner) {
                    detachedLeftEdgeDot = true;
                }
            }
            if (insideLeftEnvelope || detachedLeftEdgeDot) {
                ci.isNeighbor = true;
                continue;
            }

            // 2b) 右侧独立点画（完全对称）：
            const bool insideRightEnvelope =
                (rightInvadeMinX < inX1 && ci.right >= rightInvadeMinX && ci.left >= rightInvadeMinX - 2);
            bool detachedRightEdgeDot = false;
            if (!insideRightEnvelope && nearestExtRightCol >= 0 &&
                ci.right >= inX1 - 2 && ci.left > inX0 + (safeRoi.width * 58) / 100) {
                int distToExtRight = nearestExtRightCol - (ci.right - 1);
                int distToPrevInner = -1;
                for (int c = ci.left - 1; c >= (std::max)(inX0, ci.left - maxGapScan - 4); --c) {
                    if (obsColInk[static_cast<size_t>(c)] > 0) {
                        distToPrevInner = ci.left - c;
                        break;
                    }
                }
                if (distToPrevInner >= 2 && distToExtRight <= 4 && distToExtRight <= distToPrevInner) {
                    detachedRightEdgeDot = true;
                }
            }
            if (insideRightEnvelope || detachedRightEdgeDot) {
                ci.isNeighbor = true;
            }
        }
    }

    // 汇总被保护像素并执行最终安全锁（绝不把超过 72% 的选区目标像素误锁掉）
    int protectedInnerCnt = 0;
    bool anyNeighbor = false;
    bool anyFused = false;
    for (int id = 1; id < numLabels; ++id) {
        const auto& ci = comps[static_cast<size_t>(id)];
        if (ci.isNeighbor) {
            anyNeighbor = true;
            protectedInnerCnt += ci.insideCnt;
        }
        if (ci.isFusedBoundary) {
            anyFused = true;
        }
    }
    if (!anyNeighbor && !anyFused) return res;
    if (protectedInnerCnt * 100 >= totalInnerFgPixels * 72) {
        // 触发过度保护安全阀，清空整块保护，仅允许粘连谷底兜底
        for (int id = 1; id < numLabels; ++id) comps[static_cast<size_t>(id)].isNeighbor = false;
        anyNeighbor = false;
    }

    cv::Mat rawProtect(res.obsRoi.height, res.obsRoi.width, CV_8UC1, cv::Scalar(0));
    cv::Mat targetCore(res.obsRoi.height, res.obsRoi.width, CV_8UC1, cv::Scalar(0));

    for (int r = 0; r < res.obsRoi.height; ++r) {
        const int* lRow = labels.ptr<int>(r);
        uint8_t* pRow = rawProtect.ptr<uint8_t>(r);
        uint8_t* tRow = targetCore.ptr<uint8_t>(r);
        const bool yIn = (r >= inY0 && r < inY1);
        for (int c = 0; c < res.obsRoi.width; ++c) {
            const int id = lRow[c];
            if (id <= 0) continue;
            const auto& ci = comps[static_cast<size_t>(id)];
            if (ci.isNeighbor) {
                pRow[c] = 255;
            } else if (ci.touchesRoi && yIn && c >= inX0 && c < inX1 && !ci.isFusedBoundary) {
                tRow[c] = 255;
            }
        }
    }

    // Pass 2.5：低分物理粘连兜底（列投影谷底硬切）——只在相邻字和目标字像素直接连成一块时触发
    if (anyFused) {
        const int maxCut = std::clamp(safeRoi.height / 3, 2, (std::max)(2, safeRoi.width / 4));
        for (int id = 1; id < numLabels; ++id) {
            const auto& ci = comps[static_cast<size_t>(id)];
            if (!ci.isFusedBoundary) continue;
            if (ci.leftOutCnt >= 4) {
                int bestCol = inX0;
                int minCnt = INT_MAX;
                const int scanEnd = (std::min)(inX1 - 1, inX0 + maxCut);
                for (int c = inX0; c <= scanEnd; ++c) {
                    int cnt = 0;
                    for (int r = inY0; r < inY1; ++r) {
                        if (labels.at<int>(r, c) == id) ++cnt;
                    }
                    if (cnt < minCnt) {
                        minCnt = cnt;
                        bestCol = c;
                    }
                }
                if (bestCol > inX0) {
                    for (int r = inY0; r < inY1; ++r) {
                        const int* lRow = labels.ptr<int>(r);
                        uint8_t* pRow = rawProtect.ptr<uint8_t>(r);
                        for (int c = inX0; c < bestCol; ++c) {
                            if (lRow[c] == id) pRow[c] = 255;
                        }
                    }
                    anyNeighbor = true;
                }
            }
            if (ci.rightOutCnt >= 4) {
                int bestCol = inX1 - 1;
                int minCnt = INT_MAX;
                const int scanStart = (std::max)(inX0, inX1 - 1 - maxCut);
                for (int c = inX1 - 1; c >= scanStart; --c) {
                    int cnt = 0;
                    for (int r = inY0; r < inY1; ++r) {
                        if (labels.at<int>(r, c) == id) ++cnt;
                    }
                    if (cnt < minCnt) {
                        minCnt = cnt;
                        bestCol = c;
                    }
                }
                if (bestCol < inX1 - 1) {
                    for (int r = inY0; r < inY1; ++r) {
                        const int* lRow = labels.ptr<int>(r);
                        uint8_t* pRow = rawProtect.ptr<uint8_t>(r);
                        for (int c = bestCol + 1; c < inX1; ++c) {
                            if (lRow[c] == id) pRow[c] = 255;
                        }
                    }
                    anyNeighbor = true;
                }
            }
        }
    }

    if (!anyNeighbor) return res;

    // Pass 3：为邻字像素披上 1px 抗锯齿十字护城河（且严格避让已确认为目标字的核心像素）
    cv::Mat dilatedProtect;
    const cv::Mat crossKernel = cv::getStructuringElement(cv::MORPH_CROSS, cv::Size(3, 3));
    cv::dilate(rawProtect, dilatedProtect, crossKernel);
    for (int r = 0; r < res.obsRoi.height; ++r) {
        uint8_t* dRow = dilatedProtect.ptr<uint8_t>(r);
        const uint8_t* tRow = targetCore.ptr<uint8_t>(r);
        for (int c = 0; c < res.obsRoi.width; ++c) {
            if (tRow[c] > 0) dRow[c] = 0;
        }
    }

    res.protectMaskInObs = std::move(dilatedProtect);
    res.hasProtected = (cv::countNonZero(res.protectMaskInObs(cv::Rect(inX0, inY0, safeRoi.width, safeRoi.height))) > 0);
    if (res.hasProtected) {
        // 统计选区右半段被保护邻居像素的最左 X（用于同行右侧顺延完整囊括被保全的右邻左偏旁）
        const int rightHalfStart = inX0 + safeRoi.width / 2;
        for (int c = rightHalfStart; c < inX1; ++c) {
            bool hasP = false;
            for (int r = inY0; r < inY1; ++r) {
                if (res.protectMaskInObs.at<uint8_t>(r, c) > 0) { hasP = true; break; }
            }
            if (hasP) {
                res.rightProtectedMinX = res.obsRoi.x + c;
                break;
            }
        }
    }
    return res;
}
const wchar_t* resolveFontFamilyName(TextReplaceFontClass fc) noexcept {
    switch (fc) {
        case TextReplaceFontClass::Monospace: return L"Cascadia Mono";
        case TextReplaceFontClass::Serif:     return L"SimSun";
        case TextReplaceFontClass::UiSans:
        default:                              return L"Microsoft YaHei UI";
    }
}

const wchar_t* resolveFallbackFontFamilyName(TextReplaceFontClass fc) noexcept {
    switch (fc) {
        case TextReplaceFontClass::Monospace: return L"Consolas";
        case TextReplaceFontClass::Serif:     return L"Georgia";
        case TextReplaceFontClass::UiSans:
        default:                              return L"Segoe UI";
    }
}

DWRITE_FONT_WEIGHT toDWriteWeight(int weight) noexcept {
    if (weight >= 650) return DWRITE_FONT_WEIGHT_BOLD;
    if (weight >= 550) return DWRITE_FONT_WEIGHT_SEMI_BOLD;
    if (weight >= 460) return DWRITE_FONT_WEIGHT_MEDIUM;
    return DWRITE_FONT_WEIGHT_REGULAR;
}

/// 线程局部复用 DirectWrite / Direct2D / WIC 工厂，避免每次改字重复 COM 初始化（提速 5x+）
struct ThreadTextFactories {
    ComPtr<IDWriteFactory> dwrite;
    ComPtr<ID2D1Factory> d2d;
    ComPtr<IWICImagingFactory> wic;
    bool initialized = false;

    bool ensure() noexcept {
        if (initialized) return (dwrite && d2d && wic);
        initialized = true;
        ensureComInitOnThread();
        DWriteCreateFactory(DWRITE_FACTORY_TYPE_SHARED, __uuidof(IDWriteFactory),
                            reinterpret_cast<IUnknown**>(dwrite.GetAddressOf()));
        D2D1CreateFactory(D2D1_FACTORY_TYPE_SINGLE_THREADED, d2d.GetAddressOf());
        CoCreateInstance(CLSID_WICImagingFactory, nullptr, CLSCTX_INPROC_SERVER,
                         IID_PPV_ARGS(wic.GetAddressOf()));
        return (dwrite && d2d && wic);
    }
};

ThreadTextFactories& threadFactories() noexcept {
    thread_local ThreadTextFactories s_f;
    s_f.ensure();
    return s_f;
}

/// 构造带感知墨水微调的 DirectWrite 渲染参数（弥补 Microsoft YaHei UI 缺少 500/600 实体字重，且对齐深/浅色屏幕 Gamma）
ComPtr<IDWriteRenderingParams> createPerceptualRenderingParams(IDWriteFactory* dwrite,
                                                               int fontWeight,
                                                               bool darkBackground) {
    ComPtr<IDWriteRenderingParams> params;
    if (!dwrite) return params;
    // 暗底浅字在 Windows/Chromium Skia 下需要更柔和的 Gamma，浅底深字保持标准清锐对比度；
    // 500 Medium / 600 SemiBold 在缺少实体中字重的字体上通过温和提升 enhancedContrast 实现真实视觉加粗阶梯
    float gamma = darkBackground ? 1.55f : 1.80f;
    float enhancedContrast = darkBackground ? 0.35f : 0.50f;
    if (fontWeight >= 550 && fontWeight < 650) {
        enhancedContrast = darkBackground ? 0.85f : 1.00f;
        gamma = darkBackground ? 1.35f : 1.60f;
    } else if (fontWeight >= 460 && fontWeight < 550) {
        enhancedContrast = darkBackground ? 0.60f : 0.75f;
        gamma = darkBackground ? 1.45f : 1.70f;
    }
    dwrite->CreateCustomRenderingParams(
        gamma,
        enhancedContrast,
        0.0f, // Grayscale 亚像素平滑（无彩边干扰）
        DWRITE_PIXEL_GEOMETRY_FLAT,
        DWRITE_RENDERING_MODE_NATURAL_SYMMETRIC,
        params.GetAddressOf());
    return params;
}

ComPtr<IDWriteTextLayout> buildTextLayout(IDWriteFactory* dwrite,
                                          const std::wstring& text,
                                          TextReplaceFontClass fontClass,
                                          float fontSizePx,
                                          int fontWeight,
                                          float* outBaseline = nullptr,
                                          float* outAdvanceW = nullptr,
                                          float* outLayoutH = nullptr) {
    ComPtr<IDWriteTextLayout> layout;
    if (!dwrite) return layout;

    ComPtr<IDWriteTextFormat> fmt;
    const wchar_t* fam = resolveFontFamilyName(fontClass);
    if (FAILED(dwrite->CreateTextFormat(
            fam, nullptr, toDWriteWeight(fontWeight),
            DWRITE_FONT_STYLE_NORMAL, DWRITE_FONT_STRETCH_NORMAL,
            fontSizePx, L"zh-CN", fmt.GetAddressOf())) || !fmt) {
        dwrite->CreateTextFormat(
            resolveFallbackFontFamilyName(fontClass), nullptr, toDWriteWeight(fontWeight),
            DWRITE_FONT_STYLE_NORMAL, DWRITE_FONT_STRETCH_NORMAL,
            fontSizePx, L"zh-CN", fmt.GetAddressOf());
    }
    if (!fmt) return layout;
    fmt->SetWordWrapping(DWRITE_WORD_WRAPPING_NO_WRAP);

    const std::wstring sample = text.empty() ? L" " : text;
    if (FAILED(dwrite->CreateTextLayout(
            sample.c_str(), static_cast<UINT32>(sample.size()),
            fmt.Get(), 4096.0f, 512.0f, layout.GetAddressOf())) || !layout) {
        return {};
    }

    if (outAdvanceW || outLayoutH) {
        DWRITE_TEXT_METRICS tm{};
        if (SUCCEEDED(layout->GetMetrics(&tm))) {
            if (outAdvanceW) *outAdvanceW = text.empty() ? 2.0f : (std::max)(2.0f, tm.widthIncludingTrailingWhitespace);
            if (outLayoutH) *outLayoutH = (std::max)(fontSizePx, tm.height);
        }
    }
    if (outBaseline) {
        DWRITE_LINE_METRICS lm{};
        UINT32 lineCnt = 0;
        if (SUCCEEDED(layout->GetLineMetrics(&lm, 1, &lineCnt)) && lineCnt >= 1 && lm.baseline > 0.5f) {
            *outBaseline = lm.baseline;
        } else {
            *outBaseline = fontSizePx * 0.82f;
        }
    }
    return layout;
}

/// 真实光栅化墨水几何探针结果（Render-and-Match 物理闭环核心）
struct RenderedGlyphProbe {
    bool valid = false;
    float inkLeft = 0.0f;
    float inkRight = 0.0f;
    float inkTop = 0.0f;
    float inkBottom = 0.0f;
    float inkWidth = 0.0f;
    float inkHeight = 0.0f;
    float inkCenterY = 0.0f;
    float baseline = 0.0f;
    float advanceW = 0.0f;
    float layoutH = 0.0f;
    float alphaMass = 0.0f; // sum(alpha / 255.0)
};

struct ProbeCacheEntry {
    uint64_t key = 0;
    RenderedGlyphProbe value;
};

std::mutex g_probeCacheMutex;
std::deque<ProbeCacheEntry> g_probeCache;
constexpr size_t kMaxProbeCache = 48;

uint64_t hashProbeKey(const std::wstring& text, TextReplaceFontClass fc, float fontSizePx, int fontWeight, bool darkBg) noexcept {
    uint64_t h = 14695981039346656037ULL;
    auto mix = [&](uint64_t v) noexcept { h ^= v; h *= 1099511628211ULL; };
    mix(static_cast<uint64_t>(fc));
    mix(static_cast<uint64_t>(std::lround(fontSizePx * 4.0f)));
    mix(static_cast<uint64_t>(fontWeight));
    mix(darkBg ? 1ULL : 0ULL);
    for (wchar_t ch : text) mix(static_cast<uint64_t>(ch));
    return h;
}

RenderedGlyphProbe measureRenderedGlyphInk(const std::wstring& text,
                                           TextReplaceFontClass fontClass,
                                           float fontSizePx,
                                           int fontWeight,
                                           bool darkBackground) {
    RenderedGlyphProbe probe{};
    if (text.empty() || fontSizePx < 6.0f) return probe;

    const uint64_t key = hashProbeKey(text, fontClass, fontSizePx, fontWeight, darkBackground);
    {
        std::lock_guard<std::mutex> lock(g_probeCacheMutex);
        for (const auto& entry : g_probeCache) {
            if (entry.key == key) return entry.value;
        }
    }

    auto& fac = threadFactories();
    if (!fac.dwrite || !fac.d2d || !fac.wic) return probe;

    float base = fontSizePx * 0.82f;
    float advW = fontSizePx;
    float layH = fontSizePx * 1.25f;
    auto layout = buildTextLayout(fac.dwrite.Get(), text, fontClass, fontSizePx, fontWeight, &base, &advW, &layH);
    if (!layout) return probe;

    probe.baseline = base;
    probe.advanceW = advW;
    probe.layoutH = layH;

    const int pad = 4;
    const int bmpW = std::clamp(static_cast<int>(std::ceil(advW)) + pad * 2, 8, 1200);
    const int bmpH = std::clamp(static_cast<int>(std::ceil(layH)) + pad * 2, 8, 256);

    ComPtr<IWICBitmap> bmp;
    if (FAILED(fac.wic->CreateBitmap(static_cast<UINT>(bmpW), static_cast<UINT>(bmpH),
                                     GUID_WICPixelFormat32bppPBGRA, WICBitmapCacheOnDemand,
                                     bmp.GetAddressOf())) || !bmp) {
        return probe;
    }

    const D2D1_RENDER_TARGET_PROPERTIES rtProps = D2D1::RenderTargetProperties(
        D2D1_RENDER_TARGET_TYPE_SOFTWARE,
        D2D1::PixelFormat(DXGI_FORMAT_B8G8R8A8_UNORM, D2D1_ALPHA_MODE_PREMULTIPLIED),
        96.0f, 96.0f);
    ComPtr<ID2D1RenderTarget> rt;
    if (FAILED(fac.d2d->CreateWicBitmapRenderTarget(bmp.Get(), rtProps, rt.GetAddressOf())) || !rt) {
        return probe;
    }
    rt->SetTextAntialiasMode(D2D1_TEXT_ANTIALIAS_MODE_GRAYSCALE);
    if (auto rp = createPerceptualRenderingParams(fac.dwrite.Get(), fontWeight, darkBackground)) {
        rt->SetTextRenderingParams(rp.Get());
    }

    ComPtr<ID2D1SolidColorBrush> whiteBrush;
    rt->CreateSolidColorBrush(D2D1::ColorF(1.0f, 1.0f, 1.0f, 1.0f), whiteBrush.GetAddressOf());
    rt->BeginDraw();
    rt->Clear(D2D1::ColorF(0, 0, 0, 0));
    if (whiteBrush) {
        rt->DrawTextLayout(D2D1::Point2F(static_cast<float>(pad), static_cast<float>(pad)),
                           layout.Get(), whiteBrush.Get(), D2D1_DRAW_TEXT_OPTIONS_NONE);
    }
    if (FAILED(rt->EndDraw())) return probe;

    WICRect rc{0, 0, bmpW, bmpH};
    ComPtr<IWICBitmapLock> lock;
    if (FAILED(bmp->Lock(&rc, WICBitmapLockRead, lock.GetAddressOf())) || !lock) return probe;

    UINT stride = 0, bufSize = 0;
    BYTE* ptr = nullptr;
    if (FAILED(lock->GetStride(&stride)) || FAILED(lock->GetDataPointer(&bufSize, &ptr)) || !ptr) return probe;

    int minX = bmpW, maxX = -1, minY = bmpH, maxY = -1;
    double alphaSum = 0.0;
    for (int y = 0; y < bmpH; ++y) {
        const auto* row = reinterpret_cast<const cv::Vec4b*>(ptr + static_cast<size_t>(y) * stride);
        for (int x = 0; x < bmpW; ++x) {
            const uint8_t a = row[x][3];
            if (a > 0) {
                alphaSum += static_cast<double>(a) / 255.0;
            }
            // 与原图 extractStyle 的 42% 阈值保持一致，剔除最外围 1px 淡抗锯齿晕边
            if (a >= 96) {
                if (x < minX) minX = x;
                if (x > maxX) maxX = x;
                if (y < minY) minY = y;
                if (y > maxY) maxY = y;
            }
        }
    }

    if (maxX >= minX && maxY >= minY) {
        probe.valid = true;
        probe.inkLeft = static_cast<float>(minX - pad);
        probe.inkRight = static_cast<float>(maxX - pad + 1);
        probe.inkTop = static_cast<float>(minY - pad);
        probe.inkBottom = static_cast<float>(maxY - pad + 1);
        probe.inkWidth = (std::max)(1.0f, probe.inkRight - probe.inkLeft);
        probe.inkHeight = (std::max)(1.0f, probe.inkBottom - probe.inkTop);
        probe.inkCenterY = (probe.inkTop + probe.inkBottom) * 0.5f;
        probe.alphaMass = static_cast<float>(alphaSum);
    }

    {
        std::lock_guard<std::mutex> lockGuard(g_probeCacheMutex);
        g_probeCache.push_front(ProbeCacheEntry{key, probe});
        while (g_probeCache.size() > kMaxProbeCache) g_probeCache.pop_back();
    }
    return probe;
}
} // namespace

// ─────────────────────────────────────────────────────────────────────────────
// LinearBgPlane
// ─────────────────────────────────────────────────────────────────────────────
cv::Vec3b LinearBgPlane::evalAt(float x, float y) const noexcept {
    const float dx = x - originX;
    const float dy = y - originY;
    auto evalCh = [&](int ch) noexcept -> uint8_t {
        const float v = c0[ch] + cx[ch] * dx + cy[ch] * dy;
        return static_cast<uint8_t>(std::clamp(std::lround(v), 0L, 255L));
    };
    return cv::Vec3b(evalCh(0), evalCh(1), evalCh(2));
}
ExtractedTextStyle TextReplaceEngine::extractStyle(const cv::Mat& canvas,
                                                   const cv::Rect& sourceRect,
                                                   const std::vector<cv::Rect>& charBoxes,
                                                   const std::string& originalTextUtf8) {
    ExtractedTextStyle st{};
    if (canvas.empty() || sourceRect.width <= 0 || sourceRect.height <= 0) return st;

    const cv::Mat bgr = ensureBgr3Mat(canvas);
    const cv::Rect roi = sourceRect & cv::Rect(0, 0, bgr.cols, bgr.rows);
    if (roi.width <= 0 || roi.height <= 0) return st;

    // 1. 拟合外围二维线性渐变背景平面并评估复杂度
    st.bgPlane = fitRobustLinearBgPlane(bgr, roi);
    const cv::Vec3b centerBg = st.bgPlane.evalAt(st.bgPlane.originX, st.bgPlane.originY);
    const float bgLum = (0.114f * centerBg[0] + 0.587f * centerBg[1] + 0.299f * centerBg[2]) / 255.0f;
    st.darkBackground = (bgLum < 0.42f);
    st.complexBackground = (st.bgPlane.complexity > 11.5f);

    // 2. 提取前景字形对比度图与主体核心字色（避开边缘半透明抗锯齿，同时防止深底抓到单个极端刺眼过曝白点）
    cv::Mat diffMap(roi.height, roi.width, CV_8UC1, cv::Scalar(0));
    struct FgCandidate {
        int diff;
        cv::Vec3b bgr;
    };
    std::vector<FgCandidate> fgCands;
    fgCands.reserve(static_cast<size_t>(roi.width * roi.height / 3));

    int maxDiff = 0;
    for (int r = 0; r < roi.height; ++r) {
        const auto* row = bgr.ptr<cv::Vec3b>(roi.y + r) + roi.x;
        uint8_t* dRow = diffMap.ptr<uint8_t>(r);
        for (int c = 0; c < roi.width; ++c) {
            const cv::Vec3b bg = st.bgPlane.evalAt(static_cast<float>(roi.x + c), static_cast<float>(roi.y + r));
            const int d = (std::max)({
                std::abs(static_cast<int>(row[c][0]) - static_cast<int>(bg[0])),
                std::abs(static_cast<int>(row[c][1]) - static_cast<int>(bg[1])),
                std::abs(static_cast<int>(row[c][2]) - static_cast<int>(bg[2]))
            });
            dRow[c] = static_cast<uint8_t>((std::min)(255, d));
            if (d > maxDiff) maxDiff = d;
            if (d >= 24) {
                fgCands.push_back({d, row[c]});
            }
        }
    }

    int coreDiffRef = (std::max)(32, maxDiff);
    if (!fgCands.empty()) {
        const int coreCut = (std::max)(24, (maxDiff * (st.darkBackground ? 56 : 60)) / 100);
        std::vector<uint8_t> tb, tg, tr;
        std::vector<int> coreDiffs;
        tb.reserve(fgCands.size());
        tg.reserve(fgCands.size());
        tr.reserve(fgCands.size());
        coreDiffs.reserve(fgCands.size());
        for (const auto& fc : fgCands) {
            if (fc.diff >= coreCut) {
                tb.push_back(fc.bgr[0]);
                tg.push_back(fc.bgr[1]);
                tr.push_back(fc.bgr[2]);
                coreDiffs.push_back(fc.diff);
            }
        }
        if (tb.size() < 4) {
            std::sort(fgCands.begin(), fgCands.end(), [](const FgCandidate& a, const FgCandidate& b) {
                return a.diff > b.diff;
            });
            tb.clear(); tg.clear(); tr.clear(); coreDiffs.clear();
            const size_t topN = (std::max<size_t>)(4, (std::min<size_t>)(fgCands.size(), fgCands.size() / 3 + 1));
            for (size_t i = 0; i < topN; ++i) {
                tb.push_back(fgCands[i].bgr[0]);
                tg.push_back(fgCands[i].bgr[1]);
                tr.push_back(fgCands[i].bgr[2]);
                coreDiffs.push_back(fgCands[i].diff);
            }
        }
        const size_t m = tb.size() / 2;
        std::nth_element(tb.begin(), tb.begin() + static_cast<ptrdiff_t>(m), tb.end());
        std::nth_element(tg.begin(), tg.begin() + static_cast<ptrdiff_t>(m), tg.end());
        std::nth_element(tr.begin(), tr.begin() + static_cast<ptrdiff_t>(m), tr.end());
        std::nth_element(coreDiffs.begin(), coreDiffs.begin() + static_cast<ptrdiff_t>(m), coreDiffs.end());
        st.textBgr = cv::Vec3b(tb[m], tg[m], tr[m]);
        coreDiffRef = (std::max)(28, coreDiffs[m]);
    } else {
        st.textBgr = st.darkBackground ? cv::Vec3b(214, 218, 224) : cv::Vec3b(30, 41, 59);
    }

    // 3. 精准校准字形物理四边 [inkLeftX, inkRightX, inkTopY, inkBottomY] 与原图总墨水质量
    const uint8_t fgThresh = static_cast<uint8_t>(std::clamp(maxDiff * 40 / 100, 22, 92));
    cv::Mat fgMask;
    cv::threshold(diffMap, fgMask, fgThresh, 255, cv::THRESH_BINARY);

    // 剔除 OCR 字符框边界偏移误包入的相邻未选中字形像素（如 "剩余5%" 中渗入左界的 "余" 字右捺/独立点），
    // 确保 inkLeftX/inkRightX/字号/字重 100% 忠实于被选目标字自身，杜绝假留白与字号虚胖
    const NeighborGlyphGuardResult styleGuard = buildNeighborGlyphGuard(bgr, roi, st.bgPlane);
    if (styleGuard.hasProtected) {
        const cv::Mat guardInRoi = styleGuard.cropTo(roi);
        cv::Mat cleanedFg = fgMask.clone();
        cleanedFg.setTo(0, guardInRoi);
        if (cv::countNonZero(cleanedFg) >= 4) {
            fgMask = std::move(cleanedFg);
            diffMap.setTo(0, guardInRoi);
        }
    }

    int inkTop = 0;
    int inkBot = roi.height - 1;
    std::vector<int> rowCounts(static_cast<size_t>(roi.height), 0);
    int maxRowCount = 0;
    for (int r = 0; r < roi.height; ++r) {
        const int cnt = cv::countNonZero(fgMask.row(r));
        rowCounts[static_cast<size_t>(r)] = cnt;
        if (cnt > maxRowCount) maxRowCount = cnt;
    }
    if (maxRowCount > 0) {
        const int minRowPixels = (std::max)(1, maxRowCount / 14);
        while (inkTop < roi.height - 1 && rowCounts[static_cast<size_t>(inkTop)] < minRowPixels) ++inkTop;
        while (inkBot > inkTop && rowCounts[static_cast<size_t>(inkBot)] < minRowPixels) --inkBot;
    }

    int inkLeft = 0;
    int inkRight = roi.width - 1;
    std::vector<int> colCounts(static_cast<size_t>(roi.width), 0);
    int maxColCount = 0;
    for (int c = 0; c < roi.width; ++c) {
        const int cnt = cv::countNonZero(fgMask.col(c));
        colCounts[static_cast<size_t>(c)] = cnt;
        if (cnt > maxColCount) maxColCount = cnt;
    }
    if (maxColCount > 0) {
        const int minColPixels = (std::max)(1, maxColCount / 14);
        while (inkLeft < roi.width - 1 && colCounts[static_cast<size_t>(inkLeft)] < minColPixels) ++inkLeft;
        while (inkRight > inkLeft && colCounts[static_cast<size_t>(inkRight)] < minColPixels) --inkRight;
    }

    double actualInkMass = 0.0;
    const float invCoreDiff = 1.0f / static_cast<float>(coreDiffRef);
    for (int r = inkTop; r <= inkBot; ++r) {
        const uint8_t* dRow = diffMap.ptr<uint8_t>(r);
        for (int c = inkLeft; c <= inkRight; ++c) {
            if (dRow[c] >= 16) {
                actualInkMass += static_cast<double>(std::clamp(static_cast<float>(dRow[c]) * invCoreDiff, 0.0f, 1.0f));
            }
        }
    }
    st.inkCoverageSum = static_cast<float>(actualInkMass);

    const std::wstring wOrig = tools3000::core::WinUtils::utf8ToWstring(originalTextUtf8);
    const bool hasCjk = containsCjkCodepoint(wOrig);
    const bool hasAsc = containsAsciiAscenderOrDigit(wOrig);
    int baseRow = inkBot;
    if (!hasCjk && (inkBot - inkTop) >= 8) {
        const int denseThresh = (std::max)(2, maxRowCount / 4);
        int denseBot = inkBot;
        while (denseBot > inkTop + (inkBot - inkTop) / 2 && rowCounts[static_cast<size_t>(denseBot)] < denseThresh) {
            --denseBot;
        }
        baseRow = denseBot;
    }

    st.inkLeftX = static_cast<float>(roi.x + inkLeft);
    st.inkRightX = static_cast<float>(roi.x + inkRight + 1);
    st.inkTopY = static_cast<float>(roi.y + inkTop);
    st.inkBottomY = static_cast<float>(roi.y + inkBot + 1);
    st.baselineY = static_cast<float>(roi.y + baseRow + 1);

    // 4. 先推断字体族（检测 ASCII 字符框宽度一致性 -> 提前锁定 Monospace，供 Render-and-Match 闭环使用）
    if (!charBoxes.empty()) {
        std::vector<float> widths;
        widths.reserve(charBoxes.size());
        float advSum = 0.0f;
        int advCnt = 0;
        for (size_t i = 0; i < charBoxes.size(); ++i) {
            if (charBoxes[i].width > 1) {
                widths.push_back(static_cast<float>(charBoxes[i].width));
            }
            if (i + 1 < charBoxes.size()) {
                const float adv = static_cast<float>(charBoxes[i + 1].x - charBoxes[i].x);
                if (adv > 1.0f && adv < static_cast<float>(roi.height) * 2.5f) {
                    advSum += adv;
                    ++advCnt;
                }
            }
        }
        if (advCnt > 0) st.avgCharAdvancePx = advSum / static_cast<float>(advCnt);
        if (!hasCjk && widths.size() >= 4) {
            const float mean = std::accumulate(widths.begin(), widths.end(), 0.0f) / static_cast<float>(widths.size());
            float varSum = 0.0f;
            for (float w : widths) varSum += (w - mean) * (w - mean);
            const float cvVal = std::sqrt(varSum / static_cast<float>(widths.size())) / (std::max)(1.0f, mean);
            if (cvVal < 0.145f) {
                st.fontClass = TextReplaceFontClass::Monospace;
            }
        }
    }

    // 5. Render-and-Match 物理闭环反解精确字号（宽度 + 高度双约束，精度 ±0.25px，根治高估虚胖 20% 顽疾）
    const float actualInkW = (std::max)(1.0f, st.inkRightX - st.inkLeftX);
    const float actualInkH = (std::max)(6.0f, st.inkBottomY - st.inkTopY);
    const float visualInkH = (std::max)(6.0f, static_cast<float>(baseRow - inkTop + 1));

    float solvedEm = 0.0f;
    if (!wOrig.empty() && maxRowCount >= 3 && maxColCount >= 2) {
        constexpr float kRefEm = 20.0f;
        const RenderedGlyphProbe refProbe = measureRenderedGlyphInk(
            wOrig, st.fontClass, kRefEm, 400, st.darkBackground);
        if (refProbe.valid && refProbe.inkHeight >= 4.0f) {
            const float emFromH = kRefEm * (actualInkH / refProbe.inkHeight);
            const bool hasMultiSpaces = (wOrig.find(L"  ") != std::wstring::npos || wOrig.find(L'\t') != std::wstring::npos);
            if (wOrig.size() >= 2 && !hasMultiSpaces && refProbe.inkWidth >= 8.0f && actualInkW >= 8.0f) {
                const float emFromW = kRefEm * (actualInkW / refProbe.inkWidth);
                const float ratio = emFromW / (std::max)(1.0f, emFromH);
                if (ratio >= 0.75f && ratio <= 1.32f) {
                    // 多字符水平跨度大，抗锯齿 1px 误差占比极小，以宽度约束为主导实现像素级精确吻合
                    const float wWeight = (wOrig.size() >= 3) ? 0.68f : 0.55f;
                    solvedEm = emFromW * wWeight + emFromH * (1.0f - wWeight);
                } else {
                    solvedEm = emFromH * 0.80f + std::clamp(emFromW, emFromH * 0.78f, emFromH * 1.25f) * 0.20f;
                }
            } else {
                solvedEm = emFromH;
            }
        }
    }

    if (solvedEm <= 1.0f) {
        const float heightToEm = hasCjk ? 0.88f : (hasAsc ? 0.73f : 0.55f);
        const float estEm = visualInkH / heightToEm;
        const float rawBoxEm = static_cast<float>(roi.height) * 0.78f;
        solvedEm = (maxRowCount >= 3) ? (estEm * 0.85f + rawBoxEm * 0.15f) : rawBoxEm;
    }
    st.fontSizePx = std::clamp(std::round(solvedEm * 2.0f) * 0.5f, 9.0f, 128.0f);

    // 6. 墨水质量闭环匹配字重（实测 400/500/600/700 渲染总墨水能量，选出视觉粗细与原图最一致的字重）
    bool weightMatched = false;
    if (!wOrig.empty() && actualInkMass >= 12.0) {
        constexpr std::array<int, 4> kWeights = {{400, 500, 600, 700}};
        int bestW = 400;
        double bestErr = 1e18;
        for (int candW : kWeights) {
            const RenderedGlyphProbe wp = measureRenderedGlyphInk(
                wOrig, st.fontClass, st.fontSizePx, candW, st.darkBackground);
            if (!wp.valid || wp.alphaMass <= 1.0f) continue;
            const double err = std::abs(static_cast<double>(wp.alphaMass) - actualInkMass);
            if (err < bestErr) {
                bestErr = err;
                bestW = candW;
                weightMatched = true;
            }
        }
        if (weightMatched) {
            st.fontWeight = bestW;
        }
    }
    if (!weightMatched && cv::countNonZero(fgMask) >= 8) {
        cv::Mat distImg;
        cv::distanceTransform(fgMask, distImg, cv::DIST_L2, 3);
        std::vector<float> radii;
        radii.reserve(256);
        for (int r = 0; r < distImg.rows; ++r) {
            const float* dr = distImg.ptr<float>(r);
            for (int c = 0; c < distImg.cols; ++c) {
                if (dr[c] > 0.45f) radii.push_back(dr[c]);
            }
        }
        if (!radii.empty()) {
            const size_t p85 = (radii.size() * 85) / 100;
            std::nth_element(radii.begin(), radii.begin() + static_cast<ptrdiff_t>(p85), radii.end());
            const float strokeW = radii[p85] * 2.0f;
            const float ratio = strokeW / (std::max)(9.0f, st.fontSizePx);
            if (ratio >= 0.165f) st.fontWeight = 700;
            else if (ratio >= 0.132f) st.fontWeight = 600;
            else if (ratio >= 0.105f) st.fontWeight = 500;
            else st.fontWeight = 400;
        }
    }

    return st;
}
} // namespace tools3000::capture
