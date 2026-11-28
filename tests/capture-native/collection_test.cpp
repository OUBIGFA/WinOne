#include "capture_collection.h"
#include <iostream>
#include <stdexcept>
using namespace winone::capture;
int main() {
    try {
        CaptureCollection c;
        cv::Mat first(20, 30, CV_8UC4, cv::Scalar(1, 2, 3, 255));
        c.add(first, {10, 20, 30, 20}, 1);
        first.setTo(cv::Scalar(9, 9, 9, 255));
        c.add(cv::Mat(10, 40, CV_8UC4, cv::Scalar(4, 5, 6, 255)), {100, 200, 40, 10}, 2);
        if (c.items().size() != 2 || c.items()[0].image.at<cv::Vec4b>(0, 0)[0] != 1)
            throw std::runtime_error("fragment pixels were not retained");
        c.move(1, 0);
        c.erase(1);
        if (c.items().size() != 1 || c.items()[0].screen != 2) throw std::runtime_error("delete/reorder differs");
        c.undo();
        c.undo();
        if (c.items().size() != 2 || c.items()[0].screen != 1) throw std::runtime_error("undo did not restore order");
        c.erase(1);
        c.erase(0);
        c.undo();
        c.undo();
        if (c.items().size() != 2) throw std::runtime_error("deleting final fragment discarded undo history");
        c.replace(0, cv::Mat(20, 30, CV_8UC4, cv::Scalar(7, 8, 9, 255)), {15, 25, 30, 20});
        c.undo();
        if (c.items()[0].rect.x != 10 || c.items()[0].image.at<cv::Vec4b>(0, 0)[0] != 1)
            throw std::runtime_error("moving a selection was not reversible");
        c.clear();
        if (!c.items().empty() || c.canUndo()) throw std::runtime_error("cancel retained pixels/history");
        std::cout << "PASS: collection pixels, cross-screen order, delete-last undo, move undo and clear\n";
    } catch (const std::exception &e) {
        std::cerr << e.what() << '\n';
        return 1;
    }
}
