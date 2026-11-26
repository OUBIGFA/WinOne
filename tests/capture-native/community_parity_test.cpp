#include "draft_store.h"
#include "capture_settings.h"
#include "capture/MarkupEngine.h"
#include <opencv2/imgcodecs.hpp>
#include <iostream>
#include <fstream>

int wmain(int argc, wchar_t** argv) {
    using namespace tools3000::capture;
    try {
        if (argc != 2) throw std::invalid_argument("Pass a repository temporary PNG path");
        const std::filesystem::path destination = argv[1];
        if (!destination.is_absolute() || destination.wstring().find(L"_temp") == std::wstring::npos || std::filesystem::exists(destination))
            throw std::invalid_argument("Choose a new task-owned image path");
        if (!initializeMarkupTextRenderer()) throw std::runtime_error("Community text renderer did not initialize");
        const auto valid=winone::capture::normalizeCaptureSettings({{"outputDirectory","C:/captures"},{"toolbarTools",{"crop"}}});
        if(valid.at("toolbarTools")!=winone::capture::json::array({"crop","ocr","translate","step"})) throw std::runtime_error("Legacy toolbar did not adopt new tools exactly once");
        for(const auto& invalid:std::vector<winone::capture::json>{
            {{"outputDirectory","relative"}},{{"outputDirectory","C:/captures"},{"scrollDelay",1}},
            {{"outputDirectory","C:/captures"},{"toolbarTools",{"crop","crop"}}},
            {{"outputDirectory","C:/captures"},{"theme","unknown"}},{{"outputDirectory","C:/captures"},{"copyToClipboard","yes"}}}) {
            bool rejected=false;try{winone::capture::normalizeCaptureSettings(invalid);}catch(const std::exception&){rejected=true;}
            if(!rejected)throw std::runtime_error("Invalid capture preferences were accepted");
        }
        {
            MarkupEngine engine;
            engine.setBaseImage(cv::Mat(320, 640, CV_8UC4, cv::Scalar(245, 245, 245, 255)));
            auto* first = engine.drawNumberMark({50, 60}, MarkupColor::Red(), 1, NumberBadgeShape::Circle, NumberFormat::Arabic);
            first->fontSize = 20; first->text = "中文说明 Native caption"; first->textMaxWidth = 300;
            const auto firstId = first->id;
            auto* middle = engine.drawNumberMark({50, 160}, MarkupColor::Red(), 1, NumberBadgeShape::Circle, NumberFormat::Arabic);
            const auto middleId = middle->id;
            engine.drawNumberMark({50, 260}, MarkupColor::Red(), 1, NumberBadgeShape::Circle, NumberFormat::Arabic);
            const auto initial = engine.exportElements();
            if (initial[0].numberValue != 1 || initial[1].numberValue != 2 || initial[2].numberValue != 3)
                throw std::runtime_error("Native number sequence did not start at one");
            engine.removeElement(middleId);
            const auto removed = engine.exportElements();
            if (removed.size() != 2 || removed[1].numberValue != 2) throw std::runtime_error("Deleting a middle step left a numbering gap");
            if (!engine.undo()) throw std::runtime_error("Community undo did not restore the removed step");
            const auto restored = engine.exportElements();
            if (restored.size() != 3 || restored[2].numberValue != 3 || engine.getElementById(firstId)->text != "中文说明 Native caption")
                throw std::runtime_error("Undo changed numbering or explanation text");
            const auto rendered = engine.getCompositeImage();
            std::vector<uchar> png;
            if (!cv::imencode(".png", rendered, png)) throw std::runtime_error("Community image encoding failed");
            std::ofstream image(destination, std::ios::binary); image.write(reinterpret_cast<const char*>(png.data()), png.size());
            if (!image) throw std::runtime_error("Could not save native number rendering evidence");
            size_t badgeRed = 0, badgeWhite = 0, calloutGray = 0, calloutWhite = 0;
            for (int y = 40; y < 80; ++y) for (int x = 30; x < 70; ++x) {
                const auto pixel = rendered.at<cv::Vec4b>(y, x);
                if (pixel[2] > 220 && pixel[1] < 90 && pixel[0] < 120) ++badgeRed;
                if (pixel[0] > 240 && pixel[1] > 240 && pixel[2] > 240) ++badgeWhite;
            }
            for (int y = 35; y < 100; ++y) for (int x = 80; x < 410; ++x) {
                const auto pixel = rendered.at<cv::Vec4b>(y, x);
                if (std::abs(int(pixel[0]) - 47) <= 3 && std::abs(int(pixel[1]) - 47) <= 3 && std::abs(int(pixel[2]) - 47) <= 3) ++calloutGray;
                if (pixel[0] > 250 && pixel[1] > 250 && pixel[2] > 250) ++calloutWhite;
            }
            if (badgeRed < 200 || badgeWhite < 10 || calloutGray < 300 || calloutWhite < 10)
                throw std::runtime_error("Number explanation mismatch: badgeRed=" + std::to_string(badgeRed) +
                    " badgeWhite=" + std::to_string(badgeWhite) +
                    " calloutGray=" + std::to_string(calloutGray) +
                    " calloutWhite=" + std::to_string(calloutWhite));
        }
        shutdownMarkupTextRenderer();
        std::cout << "PASS: native number explanation rendering, middle deletion renumbering, undo and text retention\n";
        return 0;
    } catch (const std::exception& error) { std::cerr << error.what() << '\n'; return 1; }
}
