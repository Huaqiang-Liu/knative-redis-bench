不同lambda（1，10，50，100，200，500）下的最优固定等待时间，观察lambda和MaxWaitingTime的关系
36个文件：6个lambda和6个与lambda相同的等待时间，每个lambda一条线，图画出来看斜率最大的地方是不是与这条线表示的lambda相同的横坐标max waiting time t。纵坐标是P(rate<last_rate && interval<t)

算法：将unfixedWaitRandomChoice2Policy去掉抢占功能，得到的基于power of 2的，有固定最大等待时间的算法