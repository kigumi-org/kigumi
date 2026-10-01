struct BitfieldThing {
    unsigned int a : 3;
    unsigned int b : 5;
};

void bitfield_byvalue_probe(struct BitfieldThing t);
int bitfield_byvalue_add(int a, int b);
