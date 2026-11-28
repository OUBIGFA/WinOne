#pragma once
// Adapted from SnapFind SelectedRegionItem / RegionActionHistoryItem and the
// ordered staging handlers in App.xaml.cs, commit 39416254 (MIT, Loecedas 2026).
// Images own their pixels across screen changes. Unlike upstream, removing the
// last item keeps the history so Undo can still restore it.
#include <algorithm>
#include <opencv2/core.hpp>
#include <stdexcept>
#include <vector>

namespace winone::capture {
struct CaptureFragment {
    cv::Mat image;
    cv::Rect rect;
    unsigned screen = 0;
};
class CaptureCollection {
  public:
    const std::vector<CaptureFragment> &items() const {
        return items_;
    }
    bool canUndo() const {
        return !history_.empty();
    }
    size_t revision() const {
        return revision_;
    }
    void add(const cv::Mat &image, cv::Rect rect, unsigned screen) {
        if (image.empty()) throw std::invalid_argument("Empty capture fragment");
        history_.push_back({Kind::Add, items_.size(), 0, {}});
        items_.push_back({image.clone(), rect, screen});
        ++revision_;
    }
    void move(size_t from, size_t to) {
        check(from);
        check(to);
        if (from == to) return;
        auto item = items_[from];
        items_.erase(items_.begin() + from);
        items_.insert(items_.begin() + to, std::move(item));
        history_.push_back({Kind::Move, from, to, {}});
        ++revision_;
    }
    void erase(size_t index) {
        check(index);
        history_.push_back({Kind::Delete, index, 0, items_[index]});
        items_.erase(items_.begin() + index);
        ++revision_;
    }
    void replace(size_t index, const cv::Mat &image, cv::Rect rect) {
        check(index);
        if (image.empty()) throw std::invalid_argument("Empty moved capture fragment");
        history_.push_back({Kind::Replace, index, 0, items_[index]});
        items_[index] = {image.clone(), rect, items_[index].screen};
        ++revision_;
    }
    void undo() {
        if (history_.empty()) return;
        auto action = std::move(history_.back());
        history_.pop_back();
        ++revision_;
        switch (action.kind) {
        case Kind::Add:
            items_.erase(items_.begin() + action.from);
            break;
        case Kind::Delete:
            items_.insert(items_.begin() + action.from, std::move(action.item));
            break;
        case Kind::Move: {
            auto item = items_[action.to];
            items_.erase(items_.begin() + action.to);
            items_.insert(items_.begin() + action.from, std::move(item));
            break;
        }
        case Kind::Replace:
            items_[action.from] = std::move(action.item);
            break;
        }
    }
    void clear() {
        items_.clear();
        history_.clear();
        ++revision_;
    }

  private:
    enum class Kind { Add, Delete, Move, Replace };
    struct Action {
        Kind kind;
        size_t from, to;
        CaptureFragment item;
    };
    void check(size_t index) const {
        if (index >= items_.size()) throw std::out_of_range("Capture fragment no longer exists");
    }
    std::vector<CaptureFragment> items_;
    std::vector<Action> history_;
    size_t revision_ = 0;
};
} // namespace winone::capture
