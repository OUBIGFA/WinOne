#include "draft_store.h"
#include <iostream>
#include <fstream>

using namespace winone::capture;
namespace up = tools3000::capture;
int wmain(int argc, wchar_t** argv) {
    try {
        if (argc != 2) throw std::invalid_argument("Pass a unique task-owned test directory");
        const std::filesystem::path directory = argv[1];
        if (!directory.is_absolute() || directory.wstring().find(L"_temp") == std::wstring::npos || std::filesystem::exists(directory))
            throw std::invalid_argument("Choose a new repository temporary directory");
        DraftStore store(directory);
        if (!up::initializeMarkupTextRenderer()) throw std::runtime_error("Cannot initialize community text metrics");
        if (store.read()) throw std::runtime_error("Empty draft store returned a document");
        Draft draft;
        draft.id = identity(); draft.mode = "region"; draft.settings = {{"revision", 2}};
        draft.original = cv::Mat(200, 300, CV_8UC4, cv::Scalar(12, 80, 160, 255));
        draft.selection = {10, 15, 120, 80}; draft.origin = {-1920, 0};
        for (int index = 0; index <= static_cast<int>(up::MarkupTool::TextReplace); ++index) {
            up::MarkupElement element;
            element.id = static_cast<uint32_t>(index + 1); element.tool = static_cast<up::MarkupTool>(index);
            element.text = "中文 annotation"; element.fontSize = 26;
            element.startPt = {11, 23}; element.endPt = {150, 90}; element.penPoints = {{1, 2}, {20, 30}};
            element.numberValue = 4; element.isEditing = true;
            element.winoneLegacyText=element.text;element.winoneLegacyLines={{"中文",{0,0}},{"annotation",{0,31}}};
            element.winoneShadow=true;element.winoneShadowOffset={3,4};element.winoneShadowColor={10,20,30,128};
            draft.elements.push_back(element);
        }
        const auto encoded = DraftStore::encodeElements(draft.elements);
        if (DraftStore::encodeElements(DraftStore::decodeElements(encoded)) != encoded)
            throw std::runtime_error("Community annotation fields did not survive round trip");
        store.save(draft);
        {
            const auto source = directory / L"native-capture-cpp";
            std::ifstream metadata(source / L"current.json"); json saved; metadata >> saved;
            const auto oldRoot = directory / L"legacy-case" / L"native-capture";
            std::filesystem::create_directories(oldRoot / L"images");
            std::filesystem::copy_file(source / L"images" / (wide(draft.id) + L".png"), oldRoot / L"images" / (wide(draft.id) + L".png"));
            const json legacy{{"version",1},{"id",draft.id},{"revision",4},{"mode","region"},{"settings",draft.settings},
                {"width",300},{"height",200},{"imageHash",saved.at("imageSHA256")},
                {"placement",{{"x",-1920},{"y",0},{"desktopX",-1920},{"desktopY",0},{"desktopWidth",300},{"desktopHeight",200}}},
                {"document",{{"version",2},{"width",300},{"height",200},{"selection",{{"x",10},{"y",15},{"width",120},{"height",80}}},
                    {"layers",json::array({{{"kind","text"},{"bounds",{{"x",25},{"y",30},{"width",95},{"height",30}}},
                        {"text","旧版中文草稿"},{"fontPixels",20},{"strokeColor",-1},{"textColor",-1},{"points",json::array()}}})}}}};
            std::ofstream file(oldRoot / L"current.json"); file << legacy.dump(); file.close();
            const auto imported = DraftStore(directory / L"legacy-case").read();
            if (!imported || imported->id != draft.id || imported->revision != 4 || imported->origin.x != -1920 ||
                imported->elements.size()!=1 || imported->elements.front().text!="旧版中文草稿" || imported->elements.front().startPt!=cv::Point(15,15))
                throw std::runtime_error("Previous native draft lost text or physical coordinates");
            const auto pendingRoot = directory / L"pending-case" / L"native-capture";
            std::filesystem::create_directories(pendingRoot / L"images");
            auto pending = legacy;
            pending["revision"] = 0; pending["imageHash"] = "";
            pending["document"]["layers"][0]["textId"] = "12345678-1234-1234-1234-123456789abc";
            pending["text"] = {{"target", "12345678-1234-1234-1234-123456789abc"}, {"value", "尚未提交的中文"},
                {"x",25},{"y",30},{"width",95},{"height",30},{"fontPixels",20},{"colorArgb",-1},{"selectionStart",2},{"selectionLength",3}};
            std::ofstream pendingFile(pendingRoot / L"pending.json"); pendingFile << pending.dump(); pendingFile.close();
            bool incompleteRejected = false;
            try { DraftStore(directory / L"pending-case").read(); } catch (const std::exception&) { incompleteRejected = true; }
            if (!incompleteRejected) throw std::runtime_error("Incomplete previous draft was silently discarded");
            std::filesystem::copy_file(source / L"images" / (wide(draft.id) + L".png"), pendingRoot / L"images" / (wide(draft.id) + L".png"));
            const auto recovered = DraftStore(directory / L"pending-case").read();
            if (!recovered || recovered->elements.front().text != "尚未提交的中文" || !recovered->elements.front().isEditing ||
                recovered->elements.front().textCaret != 5 || recovered->elements.front().textSelAnchor != 2)
                throw std::runtime_error("Pending previous draft lost unfinished text or caret selection");
        }
        const auto restored = DraftStore(directory).read();
        if (!restored || restored->id != draft.id || restored->origin.x != -1920 || restored->selection != draft.selection ||
            DraftStore::encodeElements(restored->elements) != encoded || cv::norm(restored->original, draft.original, cv::NORM_INF) != 0)
            throw std::runtime_error("Capture draft changed across store restart");
        draft.revision = 1; draft.elements.front().text = "Changed after restart";
        store.save(draft);
        bool staleRejected = false;
        auto stale = draft; stale.revision = 0;
        try { store.save(stale); } catch (const std::exception&) { staleRejected = true; }
        if (!staleRejected || store.read()->revision != 1) throw std::runtime_error("Stale writer replaced newer draft");
        staleRejected = false;
        try { store.retire(draft.id, 0); } catch (const std::exception&) { staleRejected = true; }
        if (!staleRejected || !store.read()) throw std::runtime_error("Stale completion retired a changed draft");
        auto malformed = encoded; malformed[0]["thickness"] = -1;
        bool malformedRejected = false;
        try { DraftStore::decodeElements(malformed); } catch (const std::exception&) { malformedRejected = true; }
        if (!malformedRejected) throw std::runtime_error("Invalid annotation accepted");
        store.retire(draft.id, 1);
        if (DraftStore(directory).read()) throw std::runtime_error("Completed draft reappeared after restart");
        staleRejected = false;
        try { store.save(draft); } catch (const std::exception&) { staleRejected = true; }
        if (!staleRejected) throw std::runtime_error("Completed draft was resurrected by stale writer");
        store.cleanupImage(draft.id);
        if (std::filesystem::exists(directory / L"native-capture-cpp/images" / (wide(draft.id) + L".png")))
            throw std::runtime_error("Retired native image cache was not reclaimed");
        draft.id=identity();draft.revision=0;store.save(draft);
        store.markRetired(draft.id,0);
        if(DraftStore(directory).read()) throw std::runtime_error("Cancelled generation revived before background retirement finished");
        DraftStore(directory).recoverRetirements();
        if(DraftStore(directory).read()) throw std::runtime_error("Interrupted retirement was not recovered");
        draft.id=identity();store.save(draft);
        if(store.read()->id!=draft.id)throw std::runtime_error("Interrupted cancellation blocked the next screenshot");
        up::shutdownMarkupTextRenderer();
        std::cout << "PASS: complete community annotation round trip, original pixels, restart, stale revision and retirement\n";
        return 0;
    } catch (const std::exception& error) { std::cerr << error.what() << '\n'; return 1; }
}
