struct Point {
    int x;
    int y;
};

int cheader_add(int a, int b) {
    return a + b;
}

int cheader_point_sum(struct Point p) {
    return p.x + p.y;
}

enum Color { RED = 0, GREEN = 1, BLUE = 2 };
typedef enum Color Color;

Color cheader_next_color(Color c) {
    switch (c) {
        case RED: return GREEN;
        case GREEN: return BLUE;
        default: return RED;
    }
}
