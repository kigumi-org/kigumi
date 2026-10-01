typedef struct Point {
    int x;
    int y;
} Point;

typedef enum Color {
    RED = 0,
    GREEN = 1,
    BLUE = 2
} Color;

int cheader_add(int a, int b);
int cheader_point_sum(struct Point p);
Color cheader_next_color(Color c);

#define CHEADER_MAX 64
